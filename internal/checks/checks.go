package checks

import "github.com/platform41/dockguard/internal/config"

type Item struct {
	Name    string
	OK      bool
	Message string
}

type Result struct {
	OK    bool
	Items []Item
}

type Status struct {
	Ready   bool
	Summary string
	Items   []Item
}

func BuildStatus(cfg config.Config) Status {
	items := []Item{
		{
			Name:    "external mount configured",
			OK:      cfg.ExternalMountPath != "",
			Message: cfg.ExternalMountPath,
		},
		{
			Name:    "docker storage configured",
			OK:      cfg.DockerStoragePath != "",
			Message: cfg.DockerStoragePath,
		},
		{
			Name:    "settings path configured",
			OK:      cfg.DockerDesktopConfig.SettingsPath != "",
			Message: cfg.DockerDesktopConfig.SettingsPath,
		},
	}

	ready := true
	for _, item := range items {
		if !item.OK {
			ready = false
			break
		}
	}

	summary := "ready for check execution"
	if !ready {
		summary = "configuration incomplete"
	}

	return Status{
		Ready:   ready,
		Summary: summary,
		Items:   items,
	}
}

func RunPreflight(cfg config.Config) Result {
	items := []Item{
		{
			Name:    "external mount path configured",
			OK:      cfg.ExternalMountPath != "",
			Message: cfg.ExternalMountPath,
		},
		{
			Name:    "docker storage path configured",
			OK:      cfg.DockerStoragePath != "",
			Message: cfg.DockerStoragePath,
		},
		{
			Name:    "minimum free space configured",
			OK:      cfg.MinimumFreeSpaceGB > 0,
			Message: "minimum free space threshold is set",
		},
		{
			Name:    "docker desktop settings path configured",
			OK:      cfg.DockerDesktopConfig.SettingsPath != "",
			Message: cfg.DockerDesktopConfig.SettingsPath,
		},
	}

	ok := true
	for _, item := range items {
		if !item.OK {
			ok = false
			break
		}
	}

	return Result{
		OK:    ok,
		Items: items,
	}
}
