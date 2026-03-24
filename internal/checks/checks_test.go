package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
)

func TestRunPreflightSuccess(t *testing.T) {
	mountPath := t.TempDir()
	storagePath := filepath.Join(mountPath, "DockerDesktop")
	settingsPath := filepath.Join(t.TempDir(), "settings-store.json")

	if err := os.Mkdir(storagePath, 0o755); err != nil {
		t.Fatalf("mkdir storage path: %v", err)
	}

	content := `{"diskImageLocation": "` + storagePath + `"}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	cfg := config.Config{
		ExternalMountPath:  mountPath,
		DockerStoragePath:  storagePath,
		MinimumFreeSpaceGB: 1,
		DockerDesktopConfig: config.DockerDesktopConfig{
			SettingsPath: settingsPath,
		},
	}

	result := RunPreflight(cfg)
	if !result.OK {
		t.Fatalf("RunPreflight() OK = false, items = %#v", result.Items)
	}
}

func TestRunPreflightFailsWhenSettingsDoNotMatch(t *testing.T) {
	mountPath := t.TempDir()
	storagePath := filepath.Join(mountPath, "DockerDesktop")
	settingsPath := filepath.Join(t.TempDir(), "settings-store.json")

	if err := os.Mkdir(storagePath, 0o755); err != nil {
		t.Fatalf("mkdir storage path: %v", err)
	}

	content := `{"diskImageLocation": "/Volumes/OtherDrive/DockerDesktop"}`
	if err := os.WriteFile(settingsPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	cfg := config.Config{
		ExternalMountPath:  mountPath,
		DockerStoragePath:  storagePath,
		MinimumFreeSpaceGB: 1,
		DockerDesktopConfig: config.DockerDesktopConfig{
			SettingsPath: settingsPath,
		},
	}

	result := RunPreflight(cfg)
	if result.OK {
		t.Fatal("RunPreflight() OK = true, want false")
	}

	foundMismatch := false
	for _, item := range result.Items {
		if item.Name == "docker desktop settings match storage path" && !item.OK && strings.Contains(item.Message, "expected path not found") {
			foundMismatch = true
		}
	}

	if !foundMismatch {
		t.Fatalf("expected mismatch item, items = %#v", result.Items)
	}
}
