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
func probe() (snapshot, error) {
	output, code, err := run("container", "system", "status")
	if err != nil {
		return snapshot{}, fmt.Errorf("container system status: %w", err)
	}
	if code != 0 {
		if strings.Contains(output, "not running") {
			return snapshot{systemUp: false}, nil
		}
		return snapshot{}, fmt.Errorf("container system status: exit %d: %s", code, strings.TrimSpace(output))
	}

	output, code, err = run("container", "list", "--all", "--format", "json")
	if err != nil || code != 0 {
		return snapshot{}, fmt.Errorf("container list: exit %d: %s %v", code, strings.TrimSpace(output), errOrEmpty(err))
	}
	var list []struct {
		ID     string `json:"id"`
		Status struct {
			State string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return snapshot{}, fmt.Errorf("container list: can't read its JSON: %w", err)
	}

	snap := snapshot{systemUp: true, running: map[string]bool{}}
	for _, c := range list {
		// Anything other than "stopped" (running, starting, stopping...) may be using the drive.
		if c.Status.State != "stopped" {
			snap.running[c.ID] = true
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
	snap, err := probe()
	if err != nil {
		return false, fmt.Errorf("can't tell whether %s is running: %w", t.cfg.Name, err)
	}

	stopped := false
	if t.stackState(snap) != target.Stopped {
		output, code, err := runDetached(t.cfg.Container.Down)
		if err != nil || code != 0 {
			return false, fmt.Errorf("stop %s: %s exited %d: %s %s", t.cfg.Name, t.cfg.Container.Down, code, strings.TrimSpace(output), errOrEmpty(err))
		}
		stopped = true
	}

	if !t.cfg.Container.StopSystem {
		return stopped, nil
	}
	// Re-probe: down may have stopped containers, and only the current state counts.
	snap, err = probe()
	if err != nil {
		return stopped, fmt.Errorf("can't read the container system after stopping %s: %w", t.cfg.Name, err)
	}
	if !snap.systemUp {
		return stopped, nil
	}
	if others := t.otherRunning(snap); len(others) > 0 {
		return stopped, fmt.Errorf("not stopping the container system: other containers are running (%s); stopping it would stop them too",
			strings.Join(others, ", "))
	}
	output, code, err := run("container", "system", "stop")
	if err != nil || code != 0 {
		return stopped, fmt.Errorf("container system stop: exit %d: %s %s", code, strings.TrimSpace(output), errOrEmpty(err))
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
