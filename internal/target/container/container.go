// Package container is the container target kind: an Apple container stack started and stopped
// with its own scripts (docs/specs/multi-target.md §4.4, D-017).
package container

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
)

// Swappable for tests.
var (
	run         = platform.Run
	runDetached = platform.RunDetached
	lookPath    = exec.LookPath
	homeDir     = os.UserHomeDir
)

// Target is an Apple container stack such as Fizzy.
type Target struct {
	cfg   config.Target
	drive string
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

// snapshot is one probe of the container system.
type snapshot struct {
	systemUp bool
	// running holds every running container on the machine, by name.
	running map[string]bool
}

// probe reads the container system's status and, when it's up, every container's state.
//
// Only a confirmed answer counts. The system is down only when its status is "unregistered": no
// launchd job exists, so nothing can be running. A registered daemon that fails its health check
// reports something else, and that stays unknown, which blocks eject.
func probe() (snapshot, error) {
	output, code, err := run("container", "system", "status", "--format", "json")
	if err != nil {
		return snapshot{}, fmt.Errorf("container system status: %w", err)
	}
	var status struct {
		Status string `json:"status"`
	}
	if jsonErr := json.Unmarshal([]byte(output), &status); jsonErr != nil {
		return snapshot{}, fmt.Errorf("container system status: exit %d, unreadable output: %s", code, strings.TrimSpace(output))
	}
	switch {
	case status.Status == "unregistered":
		return snapshot{systemUp: false}, nil
	case status.Status == "running" && code == 0:
	default:
		return snapshot{}, fmt.Errorf("container system status: exit %d, status %q", code, status.Status)
	}

	output, code, err = run("container", "list", "--all", "--format", "json")
	if err != nil || code != 0 {
		return snapshot{}, fmt.Errorf("container list: exit %d: %s %v", code, strings.TrimSpace(output), errOrEmpty(err))
	}
	// Pointers tell a missing field from an empty one: an entry without an id or a state can't be
	// matched to the config, so the whole list is treated as unreadable rather than guessed at.
	var list []struct {
		ID     *string `json:"id"`
		Status *struct {
			State *string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return snapshot{}, fmt.Errorf("container list: can't read its JSON: %w", err)
	}
	// "[]" decodes to an empty list (no containers); "null" leaves it nil, which is no answer.
	if list == nil {
		return snapshot{}, fmt.Errorf("container list: got %q instead of a list", strings.TrimSpace(output))
	}

	snap := snapshot{systemUp: true, running: map[string]bool{}}
	for i, c := range list {
		if c.ID == nil || *c.ID == "" || c.Status == nil || c.Status.State == nil || *c.Status.State == "" {
			return snapshot{}, fmt.Errorf("container list: entry %d has no id or state", i)
		}
		// Anything other than "stopped" (running, starting, stopping...) may be using the drive.
		if *c.Status.State != "stopped" {
			snap.running[*c.ID] = true
		}
	}
	return snap, nil
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// stackState is the state of the stack's own containers.
func (t *Target) stackState(snap snapshot) target.State {
	up := 0
	for _, id := range t.cfg.Container.Containers {
		if snap.running[id] {
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
	if state == target.Stopped && snap.systemUp && t.cfg.Container.StopSystem {
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

func (t *Target) Start() error {
	output, code, err := runDetached(t.cfg.Container.Up)
	if err != nil || code != 0 {
		return fmt.Errorf("start %s: %s exited %d: %s %s", t.cfg.Name, t.cfg.Container.Up, code, strings.TrimSpace(output), errOrEmpty(err))
	}
	return nil
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
	if !snap.systemUp {
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

func (t *Target) otherRunning(snap snapshot) []string {
	mine := map[string]bool{}
	for _, id := range t.cfg.Container.Containers {
		mine[id] = true
	}
	var others []string
	for id := range snap.running {
		if !mine[id] {
			others = append(others, id)
		}
	}
	sort.Strings(others)
	return others
}
