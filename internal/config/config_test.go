package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReadsConfigFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	configPath := filepath.Join(t.TempDir(), "dockguard.yaml")
	content := strings.TrimSpace(`
external_mount_path: /Volumes/ExternalDocker
docker_storage_path: /Volumes/ExternalDocker/DockerDesktop
minimum_free_space_gb: 80

docker_desktop:
  settings_path: ~/Library/Application Support/Docker/settings-store.json
  require_cli_start_support: false
  fail_if_already_running: false
`)

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ExternalMountPath != "/Volumes/ExternalDocker" {
		t.Fatalf("ExternalMountPath = %q", cfg.ExternalMountPath)
	}

	if cfg.MinimumFreeSpaceGB != 80 {
		t.Fatalf("MinimumFreeSpaceGB = %d", cfg.MinimumFreeSpaceGB)
	}

	expectedSettings := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Docker", "settings-store.json")
	if cfg.DockerDesktopConfig.SettingsPath != expectedSettings {
		t.Fatalf("SettingsPath = %q, want %q", cfg.DockerDesktopConfig.SettingsPath, expectedSettings)
	}

	if cfg.DockerDesktopConfig.RequireCLIStartSupport {
		t.Fatalf("RequireCLIStartSupport = true, want false")
	}
}

func TestLoadReturnsDefaultsWhenConfigIsMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ExternalMountPath == "" {
		t.Fatal("ExternalMountPath should be set from defaults")
	}

	if cfg.DockerDesktopConfig.SettingsPath == "" {
		t.Fatal("SettingsPath should be set from defaults")
	}
}
