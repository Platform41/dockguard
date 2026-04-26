package checks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/dockerdesktop"
)

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

var isDockerDesktopRunning = dockerdesktop.IsRunning

func BuildStatus(cfg config.Config) Status {
	result := RunPreflight(cfg)

	summary := "ready"
	if !result.OK {
		summary = "not ready"
	}

	return Status{
		Ready:   result.OK,
		Summary: summary,
		Items:   result.Items,
	}
}

func RunPreflight(cfg config.Config) Result {
	items := []Item{
		checkConfiguredPath("external mount path configured", cfg.ExternalMountPath),
		checkPathExists("external mount path exists", cfg.ExternalMountPath),
		checkConfiguredPath("docker storage path configured", cfg.DockerStoragePath),
		checkPathExists("docker storage path exists", cfg.DockerStoragePath),
		checkWritableDirectory("docker storage path writable", cfg.DockerStoragePath),
		checkMinimumFreeSpace(cfg.DockerStoragePath, cfg.MinimumFreeSpaceGB),
		checkConfiguredPath("docker desktop settings path configured", cfg.DockerDesktopConfig.SettingsPath),
		checkPathExists("docker desktop settings file exists", cfg.DockerDesktopConfig.SettingsPath),
		checkSettingsContainStoragePath(cfg.DockerDesktopConfig.SettingsPath, cfg.DockerStoragePath),
	}

	if cfg.DockerDesktopConfig.FailIfAlreadyRunning {
		items = append(items, checkDockerDesktopNotRunning())
	}

	ok := true
	for _, item := range items {
		if !item.OK {
			ok = false
		}
	}

	return Result{
		OK:    ok,
		Items: items,
	}
}

func checkConfiguredPath(name, path string) Item {
	if path == "" {
		return Item{Name: name, OK: false, Message: "missing value"}
	}

	return Item{Name: name, OK: true, Message: path}
}

func checkPathExists(name, path string) Item {
	if path == "" {
		return Item{Name: name, OK: false, Message: "missing path"}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Item{Name: name, OK: false, Message: fmt.Sprintf("not found: %s", path)}
		}
		return Item{Name: name, OK: false, Message: err.Error()}
	}

	kind := "file"
	if info.IsDir() {
		kind = "directory"
	}

	return Item{Name: name, OK: true, Message: fmt.Sprintf("%s present", kind)}
}

func checkWritableDirectory(name, path string) Item {
	if path == "" {
		return Item{Name: name, OK: false, Message: "missing path"}
	}

	info, err := os.Stat(path)
	if err != nil {
		return Item{Name: name, OK: false, Message: err.Error()}
	}

	if !info.IsDir() {
		return Item{Name: name, OK: false, Message: "path is not a directory"}
	}

	file, err := os.CreateTemp(path, ".dockguard-write-check-*")
	if err != nil {
		return Item{Name: name, OK: false, Message: err.Error()}
	}

	tempName := file.Name()
	_ = file.Close()
	_ = os.Remove(tempName)

	return Item{Name: name, OK: true, Message: "temporary file create succeeded"}
}

func checkMinimumFreeSpace(path string, minimumGB int) Item {
	if path == "" {
		return Item{Name: "minimum free space", OK: false, Message: "missing path"}
	}

	if minimumGB <= 0 {
		return Item{Name: "minimum free space", OK: false, Message: "threshold must be greater than zero"}
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return Item{Name: "minimum free space", OK: false, Message: err.Error()}
	}

	availableBytes := stat.Bavail * uint64(stat.Bsize)
	availableGB := int(availableBytes / (1024 * 1024 * 1024))
	ok := availableGB >= minimumGB

	return Item{
		Name:    "minimum free space",
		OK:      ok,
		Message: fmt.Sprintf("%d GB available, %d GB required", availableGB, minimumGB),
	}
}

func checkSettingsContainStoragePath(settingsPath, dockerStoragePath string) Item {
	if settingsPath == "" {
		return Item{Name: "docker desktop settings match storage path", OK: false, Message: "missing settings path"}
	}

	if dockerStoragePath == "" {
		return Item{Name: "docker desktop settings match storage path", OK: false, Message: "missing docker storage path"}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		return Item{Name: "docker desktop settings match storage path", OK: false, Message: err.Error()}
	}

	expected := filepath.Clean(dockerStoragePath)

	paths, err := extractSettingsPaths(content)
	if err != nil {
		return Item{Name: "docker desktop settings match storage path", OK: false, Message: fmt.Sprintf("parse settings JSON: %v", err)}
	}

	for _, candidate := range paths {
		if filepath.Clean(candidate) == expected {
			return Item{
				Name:    "docker desktop settings match storage path",
				OK:      true,
				Message: expected,
			}
		}
	}

	return Item{
		Name:    "docker desktop settings match storage path",
		OK:      false,
		Message: fmt.Sprintf("expected path not found in recognized settings keys: %s", expected),
	}
}

func checkDockerDesktopNotRunning() Item {
	running, err := isDockerDesktopRunning()
	if err != nil {
		return Item{Name: "docker desktop not already running", OK: false, Message: err.Error()}
	}

	if running {
		return Item{Name: "docker desktop not already running", OK: false, Message: "Docker Desktop is already running"}
	}

	return Item{Name: "docker desktop not already running", OK: true, Message: "not running"}
}

func extractSettingsPaths(content []byte) ([]string, error) {
	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	var paths []string
	collectSettingsPaths(data, &paths)
	return paths, nil
}

func collectSettingsPaths(value any, paths *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if pathValue, ok := nested.(string); ok && isStoragePathKey(key) {
				*paths = append(*paths, pathValue)
			}
			collectSettingsPaths(nested, paths)
		}
	case []any:
		for _, item := range typed {
			collectSettingsPaths(item, paths)
		}
	}
}

func isStoragePathKey(key string) bool {
	switch strings.ToLower(key) {
	case "diskimagelocation", "diskimagepath", "datafolder", "storagepath", "virtualmachinediskpath":
		return true
	default:
		return false
	}
}
