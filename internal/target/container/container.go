// Package container is the container target kind: an Apple container stack started and stopped
// with its own scripts (docs/specs/multi-target.md §4.4, D-017).
package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/containerstate"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
)

// Swappable for tests.
var (
	run                = platform.Run
	runDetached        = platform.RunDetached
	runDetachedContext = platform.RunDetachedContext
	lookPath           = exec.LookPath
	homeDir            = os.UserHomeDir
	now                = time.Now
	sleep              = time.Sleep
	readyPollInterval  = time.Second
)

// Target is an Apple container stack such as Fizzy.
type Target struct {
	cfg   config.Target
	drive string
	// OnDiscard is called immediately before an ephemeral target's down script runs.
	OnDiscard func(name string)
}

func New(cfg config.Target, drive string) *Target {
	return &Target{cfg: cfg, drive: drive}
}

func (t *Target) Name() string { return t.cfg.Name }

func (t *Target) Kind() string { return config.KindContainer }

func (t *Target) MainPath() string {
	if t.cfg.Container.DataPath != "" {
		return t.cfg.Container.DataPath
	}
	return strings.Join(t.cfg.Container.Containers, ", ")
}

func (t *Target) Checks() []checks.Item {
	c := t.cfg.Container
	items := []checks.Item{
		executable("up script executable", c.Up),
		executable("down script executable", c.Down),
		onPath("container CLI on the PATH"),
	}
	if c.Ready != "" {
		items = append(items, executable("ready script executable", c.Ready))
	}
	if c.DataPath != "" {
		items = append(items,
			checks.PathExists("data_path exists", c.DataPath),
			checks.UnderDrive("data_path on the drive", c.DataPath, t.drive),
		)
	}
	if home, err := homeDir(); err == nil {
		appRoot := filepath.Join(home, "Library", "Application Support", "com.apple.container")
		items = append(items, checks.UnderDrive("Apple container data folder on the drive", appRoot, t.drive))
	}
	return items
}

func executable(name, path string) checks.Item {
	info, err := os.Stat(path)
	if err != nil {
		return checks.Item{Name: name, OK: false, Message: err.Error()}
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return checks.Item{Name: name, OK: false, Message: "not an executable file: " + path}
	}
	return checks.Item{Name: name, OK: true, Message: path}
}

func onPath(name string) checks.Item {
	path, err := lookPath("container")
	if err != nil {
		return checks.Item{Name: name, OK: false, Message: err.Error()}
	}
	return checks.Item{Name: name, OK: true, Message: path}
}

// probe shares the same fail-closed system snapshot with the standalone system target.
func probe() (containerstate.Snapshot, error) { return containerstate.Probe(run) }

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// stackState is the state of the stack's own containers.
func (t *Target) stackState(snap containerstate.Snapshot) target.State {
	up := 0
	for _, id := range t.cfg.Container.Containers {
		if snap.Running[id] {
			up++
		}
	}
	switch {
	case up == 0:
		return target.Stopped
	case up == len(t.cfg.Container.Containers):
		return target.Running
	default:
		return target.Partial
	}
}

// State is the stack's state for list, status and the eject rule. With stop_system, a stopped
// stack whose system still runs is partial: the system keeps files open on the drive.
func (t *Target) State() (target.State, error) {
	snap, err := probe()
	if err != nil {
		return target.Unknown, err
	}
	state := t.stackState(snap)
	if state == target.Stopped && snap.SystemUp && t.cfg.Container.StopSystem {
		return target.Partial, nil
	}
	return state, nil
}

// GuardState is what the already-running guard checks: the stack's own containers only, so a
// container system started for something else doesn't block start.
func (t *Target) GuardState() (target.State, error) {
	snap, err := probe()
	if err != nil {
		return target.Unknown, err
	}
	return t.stackState(snap), nil
}

// NotReadyError reports that up succeeded but ready did not within its overall timeout.
type NotReadyError struct {
	Timeout time.Duration
}

func (e *NotReadyError) Error() string {
	return fmt.Sprintf("started but not ready after %s", e.Timeout)
}

func (t *Target) Start() error {
	_, err := t.StartWithReadiness()
	return err
}

// ReadinessEnabled reports whether this target has a readiness script to poll after start.
func (t *Target) ReadinessEnabled() bool {
	return t.cfg.Container.Ready != ""
}

// StartWithReadiness runs up, then polls ready once per second until success or the overall
// deadline. A not-ready result never stops the container.
func (t *Target) StartWithReadiness() (time.Duration, error) {
	c := t.cfg.Container
	output, code, err := runDetached(c.Up)
	if err != nil || code != 0 {
		return 0, fmt.Errorf("start %s: %s exited %d: %s %s", t.cfg.Name, c.Up, code, strings.TrimSpace(output), errOrEmpty(err))
	}
	if c.Ready == "" {
		return 0, nil
	}

	started := now()
	timeout := time.Duration(c.ReadyTimeoutSeconds) * time.Second
	deadline := started.Add(timeout)
	// Deadline arithmetic uses injectable now; contexts use the wall clock (identical in production).
	overall, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	for {
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return 0, &NotReadyError{Timeout: timeout}
		}
		attemptBound := min(5*time.Second, remaining)
		attempt, cancelAttempt := context.WithDeadline(overall, now().Add(attemptBound))
		_, code, err = runDetachedContext(attempt, c.Ready)
		cancelAttempt()
		if err == nil && code == 0 {
			return now().Sub(started), nil
		}

		remaining = deadline.Sub(now())
		if remaining > readyPollInterval {
			remaining = readyPollInterval
		}
		if remaining > 0 {
			sleep(remaining)
		}
	}
}

// Stop runs down when any of the stack's containers runs, then, with stop_system, stops the
// container system unless containers outside the stack are running.
func (t *Target) Stop() (bool, error) {
	stackStopped, err := t.StopStack()
	if err != nil {
		return false, err
	}
	systemStopped, err := t.StopSystem()
	return stackStopped || systemStopped, err
}

// StopStack runs down when any of the stack's containers runs. stop --all calls it for every
// stack before any StopSystem, so stacks sharing the system don't block each other's shutdown.
func (t *Target) StopStack() (bool, error) {
	snap, err := probe()
	if err != nil {
		return false, fmt.Errorf("can't tell whether %s is running: %w", t.cfg.Name, err)
	}
	if t.stackState(snap) == target.Stopped {
		return false, nil
	}
	if t.cfg.Container.Ephemeral && t.OnDiscard != nil {
		t.OnDiscard(t.cfg.Name)
	}
	output, code, err := runDetached(t.cfg.Container.Down)
	if err != nil || code != 0 {
		return false, fmt.Errorf("stop %s: %s exited %d: %s %s", t.cfg.Name, t.cfg.Container.Down, code, strings.TrimSpace(output), errOrEmpty(err))
	}
	return true, nil
}

// StopSystem stops the container system when stop_system is set and no container outside the
// stack is running. Without stop_system it does nothing.
func (t *Target) StopSystem() (bool, error) {
	if !t.cfg.Container.StopSystem {
		return false, nil
	}
	// Probe now: only the current state counts.
	snap, err := probe()
	if err != nil {
		return false, fmt.Errorf("can't read the container system for %s: %w", t.cfg.Name, err)
	}
	if !snap.SystemUp {
		return false, nil
	}
	if others := t.otherRunning(snap); len(others) > 0 {
		return false, fmt.Errorf("not stopping the container system: other containers are running (%s); stopping it would stop them too",
			strings.Join(others, ", "))
	}
	output, code, err := run("container", "system", "stop")
	if err != nil || code != 0 {
		return false, fmt.Errorf("container system stop: exit %d: %s %s", code, strings.TrimSpace(output), errOrEmpty(err))
	}
	return true, nil
}

func (t *Target) otherRunning(snap containerstate.Snapshot) []string {
	return containerstate.OtherRunning(snap, t.cfg.Container.Containers)
}
