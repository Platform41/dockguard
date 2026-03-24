package config

import "path/filepath"

type Config struct {
	ExternalMountPath   string
	DockerStoragePath   string
	MinimumFreeSpaceGB  int
	DockerDesktopConfig DockerDesktopConfig
}

type DockerDesktopConfig struct {
	SettingsPath           string
	RequireCLIStartSupport bool
	FailIfAlreadyRunning   bool
}

func LoadDefault() (Config, error) {
	home := "~"

	return Config{
		ExternalMountPath:  "/Volumes/DockerSSD",
		DockerStoragePath:  "/Volumes/DockerSSD/DockerDesktop",
		MinimumFreeSpaceGB: 50,
		DockerDesktopConfig: DockerDesktopConfig{
			SettingsPath:           filepath.Join(home, "Library", "Application Support", "Docker", "settings-store.json"),
			RequireCLIStartSupport: true,
			FailIfAlreadyRunning:   true,
		},
	}, nil
}
