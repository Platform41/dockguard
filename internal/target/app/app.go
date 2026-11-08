// Package app is the app target kind: a macOS GUI app whose data is linked onto the drive
// (docs/specs/multi-target.md §4.2).
package app

import (
	"fmt"
	"os"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
)

// Swappable for tests.
var (
	processRunning = platform.ProcessRunning
	openApp        = platform.OpenApp
	quitApp        = platform.QuitApp
)

// Target is a GUI app such as Xcode.
type Target struct {
	cfg   config.Target
	drive string
}

func New(cfg config.Target, drive string) *Target {
	return &Target{cfg: cfg, drive: drive}
}

func (t *Target) Name() string { return t.cfg.Name }

func (t *Target) Kind() string { return config.KindApp }

// MainPath shows the first linked path and where it points.
func (t *Target) MainPath() string {
	paths := t.cfg.App.SymlinkPaths
	if len(paths) == 0 {
		return "-"
	}
	if dest, err := os.Readlink(paths[0]); err == nil {
		return paths[0] + " -> " + dest
	}
	return paths[0]
}

// Checks: each linked path must reach an existing folder on the drive. A broken link fails as
// "not found"; a plain folder on the internal disk fails as "not on the drive", because it means
// the app's data was silently recreated off the drive.
func (t *Target) Checks() []checks.Item {
	var items []checks.Item
	for _, path := range t.cfg.App.SymlinkPaths {
		items = append(items,
			checks.PathExists(path+" exists", path),
			checks.UnderDrive(path+" on the drive", path, t.drive),
		)
	}
	return items
}

func (t *Target) State() (target.State, error) {
	running, err := processRunning(t.cfg.App.Process)
	if err != nil {
		return target.Unknown, err
	}
	if running {
		return target.Running, nil
	}
	return target.Stopped, nil
}

func (t *Target) Start() error { return openApp(t.cfg.App.Name) }

// Stop asks the app to quit, only once it's confirmed running, so it never launches the app just
// to quit it.
func (t *Target) Stop() (bool, error) {
	state, err := t.State()
	switch state {
	case target.Stopped:
		return false, nil
	case target.Unknown:
		return false, fmt.Errorf("can't tell whether %s is running: %w", t.cfg.App.Name, err)
	}
	if err := quitApp(t.cfg.App.Name); err != nil {
		return false, err
	}
	return true, nil
}
