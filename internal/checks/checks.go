// Package checks holds the preflight checks shared by every target kind
// (docs/specs/multi-target.md §5).
package checks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Item is the result of one check.
type Item struct {
	Name    string
	OK      bool
	Skipped bool
	Message string
}

// AllOK reports whether every item passed.
func AllOK(items []Item) bool {
	for _, item := range items {
		if !item.OK {
			return false
		}
	}
	return true
}

// SkipAll marks items as skipped because the drive isn't mounted (spec §5).
func SkipAll(items []Item) []Item {
	skipped := make([]Item, len(items))
	for i, item := range items {
		skipped[i] = Item{Name: item.Name, OK: false, Skipped: true, Message: "skipped: drive not mounted"}
	}
	return skipped
}

// Drive returns C1: the drive is configured and mounted. These keep their v0.1 names.
func Drive(path string) []Item {
	return []Item{
		ConfiguredPath("external mount path configured", path),
		PathExists("external mount path exists", path),
	}
}

// DriveMounted reports whether the drive path exists.
func DriveMounted(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ConfiguredPath fails when a required path is empty.
func ConfiguredPath(name, path string) Item {
	if path == "" {
		return Item{Name: name, OK: false, Message: "missing value"}
	}

	return Item{Name: name, OK: true, Message: path}
}

// PathExists is C2.
func PathExists(name, path string) Item {
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

// WritableDirectory is C3.
func WritableDirectory(name, path string) Item {
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

// MinimumFreeSpace is C4.
func MinimumFreeSpace(path string, minimumGB int) Item {
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

// UnderDrive is C6: the path resolves to somewhere under the drive.
func UnderDrive(name, path, drive string) Item {
	if path == "" || drive == "" {
		return Item{Name: name, OK: false, Message: "missing path"}
	}

	resolvedPath := resolve(path)
	resolvedDrive := resolve(drive)
	if isWithin(resolvedPath, resolvedDrive) {
		return Item{Name: name, OK: true, Message: resolvedPath}
	}

	return Item{
		Name:    name,
		OK:      false,
		Message: fmt.Sprintf("not on the drive: %s is outside %s", resolvedPath, resolvedDrive),
	}
}

// resolve follows symlinks. For a path that doesn't exist yet, it resolves the nearest existing
// parent and keeps the rest, so /var/x and /private/var/x compare equal on macOS.
func resolve(path string) string {
	clean := filepath.Clean(path)
	rest := ""
	for dir := clean; ; dir = filepath.Dir(dir) {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return clean
		}
		rest = filepath.Join(filepath.Base(dir), rest)
	}
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// SettingsContainStoragePath checks Docker Desktop's settings file points at the storage path.
func SettingsContainStoragePath(settingsPath, dockerStoragePath string) Item {
	const name = "docker desktop settings match storage path"
	if settingsPath == "" {
		return Item{Name: name, OK: false, Message: "missing settings path"}
	}

	if dockerStoragePath == "" {
		return Item{Name: name, OK: false, Message: "missing docker storage path"}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		return Item{Name: name, OK: false, Message: err.Error()}
	}

	expected := filepath.Clean(dockerStoragePath)

	paths, err := extractSettingsPaths(content)
	if err != nil {
		return Item{Name: name, OK: false, Message: fmt.Sprintf("parse settings JSON: %v", err)}
	}

	for _, candidate := range paths {
		if filepath.Clean(candidate) == expected {
			return Item{Name: name, OK: true, Message: expected}
		}
	}

	return Item{
		Name:    name,
		OK:      false,
		Message: fmt.Sprintf("expected path not found in recognized settings keys: %s", expected),
	}
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
