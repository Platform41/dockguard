// Package containersystem manages the Apple container system as an explicit target.
package containersystem

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/containerstate"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
)

var (
	run      = platform.Run
	lookPath = exec.LookPath
	homeDir  = os.UserHomeDir
)

type Target struct {
	cfg     config.Target
	drive   string
	managed []string
}

// managed contains container names from every configured container stack.
func New(cfg config.Target, drive string, managed []string) *Target {
	return &Target{cfg: cfg, drive: drive, managed: append([]string{}, managed...)}
}
func (t *Target) Name() string { return t.cfg.Name }
func (t *Target) Kind() string { return config.KindContainerSystem }
func (t *Target) MainPath() string {
	if t.cfg.ContainerSystem.DataPath != "" {
		return t.cfg.ContainerSystem.DataPath
	}
	home, err := homeDir()
	if err != nil {
		return "-"
	}
	return filepath.Join(home, "Library", "Application Support", "com.apple.container")
}
func (t *Target) Checks() []checks.Item {
	path, err := lookPath("container")
	cli := checks.Item{Name: "container CLI on the PATH", OK: err == nil, Message: path}
	if err != nil {
		cli.Message = err.Error()
	}
	items := []checks.Item{cli}
	if path := t.cfg.ContainerSystem.DataPath; path != "" {
		items = append(items, checks.PathExists("data_path exists", path), checks.UnderDrive("data_path on the drive", path, t.drive))
	}
	home, err := homeDir()
	if err != nil {
		return append(items, checks.Item{Name: "Apple container data folder on the drive", Message: err.Error()})
	}
	return append(items, checks.UnderDrive("Apple container data folder on the drive", filepath.Join(home, "Library", "Application Support", "com.apple.container"), t.drive))
}
func (t *Target) State() (target.State, error) {
	snap, err := containerstate.Probe(run)
	if err != nil {
		return target.Unknown, err
	}
	if snap.SystemUp {
		return target.Running, nil
	}
	return target.Stopped, nil
}
func (t *Target) Start() error {
	out, code, err := run("container", "system", "start")
	if err != nil || code != 0 {
		return fmt.Errorf("container system start: exit %d: %s %v", code, strings.TrimSpace(out), err)
	}
	state, err := t.State()
	if err != nil {
		return fmt.Errorf("confirm container system start: %w", err)
	}
	if state != target.Running {
		return fmt.Errorf("container system start: not confirmed running (state %s)", state)
	}
	return nil
}
func (t *Target) Stop() (bool, error) {
	snap, err := containerstate.Probe(run)
	if err != nil {
		return false, fmt.Errorf("can't read the container system for %s: %w", t.Name(), err)
	}
	if !snap.SystemUp {
		return false, nil
	}
	if others := containerstate.OtherRunning(snap, t.managed); len(others) > 0 {
		return false, &UnmanagedError{Names: others}
	}
	out, code, err := run("container", "system", "stop")
	if err != nil || code != 0 {
		return false, fmt.Errorf("container system stop: exit %d: %s %v", code, strings.TrimSpace(out), err)
	}
	return true, nil
}

// UnmanagedError identifies the running containers preventing a system stop.
type UnmanagedError struct {
	Names []string
}

func (e *UnmanagedError) Error() string {
	return fmt.Sprintf("not stopping the container system: unmanaged containers are running (%s); stopping it would stop them too", strings.Join(e.Names, ", "))
}

// StopUnmanaged stops only the explicitly named container. Configured containers
// are stopped through their own targets.
func (t *Target) StopUnmanaged(name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid container name %q", name)
	}
	for _, managed := range t.managed {
		if name == managed {
			return fmt.Errorf("container %s is managed by a configured target", name)
		}
	}
	out, code, err := run("container", "stop", name)
	if err != nil || code != 0 {
		return fmt.Errorf("container stop %s: exit %d: %s %v", name, code, strings.TrimSpace(out), err)
	}
	return nil
}

var _ target.Target = (*Target)(nil)
