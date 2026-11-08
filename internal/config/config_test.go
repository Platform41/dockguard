package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chdir is t.Chdir for Go 1.23.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dockguard.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadReadsCurrentFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := writeConfig(t, `
drive: /Volumes/ExternalSSD
minimum_free_space_gb: 80
targets:
  docker:
    kind: docker-desktop
    storage_path: /Volumes/ExternalSSD/docker-data/DockerDesktop
    settings_path: ~/Library/Application Support/Docker/settings-store.json
    fail_if_already_running: false
  docker-two:
    kind: docker-desktop
    storage_path: /Volumes/ExternalSSD/other
    minimum_free_space_gb: 10
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Legacy || cfg.Path != path || cfg.Drive != "/Volumes/ExternalSSD" || cfg.MinimumFreeSpaceGB != 80 {
		t.Fatalf("cfg = %#v", cfg)
	}
	if got := strings.Join(cfg.Names(), ","); got != "docker,docker-two" {
		t.Fatalf("Names() = %q, want file order", got)
	}

	docker := cfg.Targets[0]
	wantSettings := filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Docker", "settings-store.json")
	if docker.DockerDesktop.SettingsPath != wantSettings {
		t.Fatalf("SettingsPath = %q, want %q", docker.DockerDesktop.SettingsPath, wantSettings)
	}
	if docker.FailIfAlreadyRunning || !docker.DockerDesktop.RequireCLIStartSupport || docker.MinimumFreeSpaceGB != 80 {
		t.Fatalf("docker = %#v", docker)
	}
	if cfg.Targets[1].MinimumFreeSpaceGB != 10 {
		t.Fatalf("override MinimumFreeSpaceGB = %d, want 10", cfg.Targets[1].MinimumFreeSpaceGB)
	}
}

// AS-01 and AS-02: a v0.1 config still loads, as a single "docker" target.
func TestLoadReadsLegacyFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := writeConfig(t, `
external_mount_path: /Volumes/ExternalDocker
docker_storage_path: /Volumes/ExternalDocker/DockerDesktop
minimum_free_space_gb: 80

docker_desktop:
  settings_path: ~/Library/Application Support/Docker/settings-store.json
  require_cli_start_support: false
  fail_if_already_running: false
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Legacy || cfg.Drive != "/Volumes/ExternalDocker" || len(cfg.Targets) != 1 {
		t.Fatalf("cfg = %#v", cfg)
	}
	docker := cfg.Targets[0]
	if docker.Name != "docker" || docker.Kind != KindDockerDesktop || docker.MinimumFreeSpaceGB != 80 {
		t.Fatalf("docker = %#v", docker)
	}
	if docker.FailIfAlreadyRunning || docker.DockerDesktop.RequireCLIStartSupport {
		t.Fatalf("booleans not read: %#v", docker)
	}
	if docker.DockerDesktop.StoragePath != "/Volumes/ExternalDocker/DockerDesktop" {
		t.Fatalf("StoragePath = %q", docker.DockerDesktop.StoragePath)
	}
}

func TestLegacyDefaultsMatchV01(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse([]byte("minimum_free_space_gb: 60\n"))
	if err != nil {
		t.Fatal(err)
	}
	docker := cfg.Targets[0]
	if cfg.Drive != "/Volumes/DockerSSD" || docker.DockerDesktop.StoragePath != "/Volumes/DockerSSD/DockerDesktop" {
		t.Fatalf("defaults = %#v / %#v", cfg, docker)
	}
	if !docker.FailIfAlreadyRunning || !docker.DockerDesktop.RequireCLIStartSupport {
		t.Fatalf("default booleans = %#v", docker)
	}
}

func TestParseErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"mixed formats (AS-03)", `
drive: /Volumes/X
external_mount_path: /Volumes/X
targets:
  docker: {kind: docker-desktop, storage_path: /Volumes/X/d}
`, []string{"drive", "external_mount_path", "use one"}},
		{"unknown kind (AS-04)", `
drive: /Volumes/X
targets:
  db: {kind: mongodb}
`, []string{`target "db"`, `unknown kind "mongodb"`}},
		{"unknown key (AS-04)", `
drive: /Volumes/X
targets:
  docker: {kind: docker-desktop, storage_path: /Volumes/X/d, storage: /x}
`, []string{`target "docker"`, `unknown key "storage"`}},
		{"unknown top-level key", `
drive: /Volumes/X
target: {}
`, []string{`unknown key "target"`}},
		{"planned kind", `
drive: /Volumes/X
targets:
  db: {kind: postgres, data_dir: /x}
`, []string{`kind "postgres" is planned for v0.3.0`}},
		{"app without app", `
drive: /Volumes/X
targets:
  xcode: {kind: app}
`, []string{`missing required key "app"`}},
		{"app with a docker key", `
drive: /Volumes/X
targets:
  xcode: {kind: app, app: Xcode, storage_path: /x}
`, []string{`unknown key "storage_path"`}},
		{"missing kind", `
drive: /Volumes/X
targets:
  docker: {storage_path: /Volumes/X/d}
`, []string{`missing required key "kind"`}},
		{"missing storage path", `
drive: /Volumes/X
targets:
  docker: {kind: docker-desktop}
`, []string{`missing required key "storage_path"`}},
		{"missing drive", `
targets:
  docker: {kind: docker-desktop, storage_path: /d}
`, []string{`missing required key "drive"`}},
		{"bad target name", `
drive: /Volumes/X
targets:
  Docker_1: {kind: docker-desktop, storage_path: /Volumes/X/d}
`, []string{"lowercase letters"}},
		{"no targets", `
drive: /Volumes/X
targets: {}
`, []string{"at least one target"}},
		{"unknown legacy key", `
external_mount_path: /Volumes/X
docker_desktop:
  settings: /x
`, []string{"v0.1 format", "settings"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(strings.TrimSpace(tt.content)))
			if err == nil {
				t.Fatal("Parse() error = nil")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

// AS-21: with no --config and no ./dockguard.yaml, the user-level config is used.
func TestResolveFallsBackToUserConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, t.TempDir())

	userConfig := filepath.Join(home, UserConfigPath)
	if err := os.MkdirAll(filepath.Dir(userConfig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userConfig, []byte("minimum_free_space_gb: 50\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Resolve("")
	if err != nil || got != userConfig {
		t.Fatalf("Resolve() = %q, %v; want %q", got, err, userConfig)
	}

	if err := os.WriteFile(DefaultConfigPath, []byte("minimum_free_space_gb: 50\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := Resolve(""); got != DefaultConfigPath {
		t.Fatalf("Resolve() = %q, want the current folder's file first", got)
	}
}

// AS-22: no config anywhere is an error naming the paths tried.
func TestLoadFailsWhenNoConfigExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, t.TempDir())

	_, err := Load("")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), DefaultConfigPath) || !strings.Contains(err.Error(), filepath.Join(home, UserConfigPath)) {
		t.Fatalf("error %q should name both paths", err)
	}

	if _, err := Load(filepath.Join(home, "missing.yaml")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Load(explicit missing) error = %v, want ErrNotFound", err)
	}
}

func TestDetectSettingsPathUsesExistingCandidate(t *testing.T) {
	home := t.TempDir()

	settingsPath := filepath.Join(home, "Library", "Group Containers", "group.com.docker", "settings-store.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	resolved, err := detectSettingsPath(home)
	if err != nil {
		t.Fatalf("detectSettingsPath() error = %v", err)
	}

	if resolved != settingsPath {
		t.Fatalf("detectSettingsPath() = %q, want %q", resolved, settingsPath)
	}
}

func TestDetectSettingsPathFallsBackToFirstCandidate(t *testing.T) {
	home := t.TempDir()

	resolved, err := detectSettingsPath(home)
	if err != nil {
		t.Fatalf("detectSettingsPath() error = %v", err)
	}

	expected := filepath.Join(home, "Library", "Application Support", "Docker", "settings-store.json")
	if resolved != expected {
		t.Fatalf("detectSettingsPath() = %q, want %q", resolved, expected)
	}
}

func TestLoadReadsAppTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := Parse([]byte(`
drive: /Volumes/ExternalSSD
targets:
  xcode:
    kind: app
    app: Xcode
    symlink_paths:
      - ~/Library/Developer/Xcode/DerivedData
      - ~/Library/Developer/Xcode/Archives
  preview:
    kind: app
    app: Preview
    process: PreviewHelper
    fail_if_already_running: false
`))
	if err != nil {
		t.Fatal(err)
	}
	xcode := cfg.Targets[0]
	if xcode.Kind != KindApp || xcode.App.Name != "Xcode" || xcode.App.Process != "Xcode" || !xcode.FailIfAlreadyRunning {
		t.Fatalf("xcode = %#v / %#v", xcode, xcode.App)
	}
	want := filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData")
	if len(xcode.App.SymlinkPaths) != 2 || xcode.App.SymlinkPaths[0] != want {
		t.Fatalf("SymlinkPaths = %v", xcode.App.SymlinkPaths)
	}
	preview := cfg.Targets[1]
	if preview.App.Process != "PreviewHelper" || preview.FailIfAlreadyRunning {
		t.Fatalf("preview = %#v / %#v", preview, preview.App)
	}
}

// AS-14: CoreSimulator devices, anything inside them, and any folder holding them are rejected.
func TestAppRejectsCoreSimulator(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, path := range []string{
		"~/Library/Developer/CoreSimulator/Devices",
		"~/Library/Developer/CoreSimulator/Devices/ABC-123",
		"~/Library/Developer/CoreSimulator",
		"~/Library/Developer",
	} {
		_, err := Parse([]byte("drive: /Volumes/X\ntargets:\n  xcode:\n    kind: app\n    app: Xcode\n    symlink_paths: [" + path + "]\n"))
		if err == nil || !strings.Contains(err.Error(), "simctl create") {
			t.Fatalf("%s: error = %v, want the CoreSimulator reason", path, err)
		}
	}

	if _, err := Parse([]byte("drive: /Volumes/X\ntargets:\n  xcode:\n    kind: app\n    app: Xcode\n    symlink_paths: [~/Library/Developer/Xcode/DerivedData]\n")); err != nil {
		t.Fatalf("DerivedData rejected: %v", err)
	}
}
