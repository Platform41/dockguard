// Package dockerdesktop is the docker-desktop target kind (docs/specs/multi-target.md §4.1).
package dockerdesktop

import (
	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	desktop "github.com/platform41/dockguard/internal/dockerdesktop"
	"github.com/platform41/dockguard/internal/target"
)

var (
	isRunning = desktop.IsRunning
	start     = desktop.Start
	stop      = desktop.Stop
)

// Target is a Docker Desktop install whose storage is on the drive.
type Target struct {
	cfg    config.Target
	drive  string
	legacy bool
}

// New returns the target. legacy is true for a v0.1 config, which keeps v0.1's exact checks.
func New(cfg config.Target, drive string, legacy bool) *Target {
	return &Target{cfg: cfg, drive: drive, legacy: legacy}
}

func (t *Target) Name() string { return t.cfg.Name }

func (t *Target) Kind() string { return config.KindDockerDesktop }

func (t *Target) MainPath() string { return t.cfg.DockerDesktop.StoragePath }

func (t *Target) Checks() []checks.Item {
	d := t.cfg.DockerDesktop
	items := []checks.Item{
		checks.ConfiguredPath("docker storage path configured", d.StoragePath),
		checks.PathExists("docker storage path exists", d.StoragePath),
	}
	if !t.legacy {
		items = append(items, checks.UnderDrive("docker storage path on the drive", d.StoragePath, t.drive))
	}
	return append(items,
		checks.WritableDirectory("docker storage path writable", d.StoragePath),
		checks.MinimumFreeSpace(d.StoragePath, t.cfg.MinimumFreeSpaceGB),
		checks.ConfiguredPath("docker desktop settings path configured", d.SettingsPath),
		checks.PathExists("docker desktop settings file exists", d.SettingsPath),
		checks.SettingsContainStoragePath(d.SettingsPath, d.StoragePath),
	)
}

func (t *Target) State() (target.State, error) {
	running, err := isRunning()
	if err != nil {
		return target.Unknown, err
	}
	if running {
		return target.Running, nil
	}
	return target.Stopped, nil
}

func (t *Target) Start() error {
	return start(desktop.StartOptions{
		RequireCLIStartSupport: t.cfg.DockerDesktop.RequireCLIStartSupport,
		FailIfAlreadyRunning:   t.cfg.FailIfAlreadyRunning,
	})
}

func (t *Target) Stop() (bool, error) { return stop() }
