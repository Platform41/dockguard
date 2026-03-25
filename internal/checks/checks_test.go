package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
)

func TestRunPreflightSuccess(t *testing.T) {
	originalIsRunning := isDockerDesktopRunning
	t.Cleanup(func() {
		isDockerDesktopRunning = originalIsRunning
	})
	isDockerDesktopRunning = func() (bool, error) {
		return false, nil
	}

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
			SettingsPath:         settingsPath,
			FailIfAlreadyRunning: true,
		},
	}

	result := RunPreflight(cfg)
	if !result.OK {
		t.Fatalf("RunPreflight() OK = false, items = %#v", result.Items)
	}
}

func TestRunPreflightFailsWhenSettingsDoNotMatch(t *testing.T) {
	originalIsRunning := isDockerDesktopRunning
	t.Cleanup(func() {
		isDockerDesktopRunning = originalIsRunning
	})
	isDockerDesktopRunning = func() (bool, error) {
		return false, nil
	}

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

func TestRunPreflightFailsWhenDockerDesktopIsAlreadyRunning(t *testing.T) {
	originalIsRunning := isDockerDesktopRunning
	t.Cleanup(func() {
		isDockerDesktopRunning = originalIsRunning
	})
	isDockerDesktopRunning = func() (bool, error) {
		return true, nil
	}

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
			SettingsPath:         settingsPath,
			FailIfAlreadyRunning: true,
		},
	}

	result := RunPreflight(cfg)
	if result.OK {
		t.Fatal("RunPreflight() OK = true, want false")
	}

	foundRunningFailure := false
	for _, item := range result.Items {
		if item.Name == "docker desktop not already running" && !item.OK {
			foundRunningFailure = true
		}
	}

	if !foundRunningFailure {
		t.Fatalf("expected already-running failure, items = %#v", result.Items)
	}
}

func TestExtractSettingsPathsCollectsNestedKeys(t *testing.T) {
	content := []byte(`{
  "diskImageLocation": "/Volumes/External/DockerDesktop",
  "nested": {
    "dataFolder": "/Volumes/External/Data",
    "more": [{"storagePath": "/Volumes/External/Storage"}]
  }
}`)

	paths, err := extractSettingsPaths(content)
	if err != nil {
		t.Fatalf("extractSettingsPaths() error = %v", err)
	}

	seen := map[string]bool{}
	for _, path := range paths {
		seen[path] = true
	}

	expected := []string{
		"/Volumes/External/DockerDesktop",
		"/Volumes/External/Data",
		"/Volumes/External/Storage",
	}

	for _, path := range expected {
		if !seen[path] {
			t.Fatalf("expected path %q in results, got %#v", path, paths)
		}
	}
}
