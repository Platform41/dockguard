package holders

import (
	"path/filepath"
	"strings"
)

const managedRuntimeHint = "stop it through its DockGuard target or container stop"

func isManagedRuntime(path string) bool {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return false
	}
	if matchesRuntimePath(path) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(path)
	return err == nil && resolved != path && matchesRuntimePath(resolved)
}

func matchesRuntimePath(path string) bool {
	if strings.HasPrefix(path, "/System/Library/Frameworks/Virtualization.framework/") ||
		strings.Contains(path, "/Docker.app/Contents/") ||
		strings.Contains(path, "/libexec/container-plugins/") ||
		strings.Contains(path, "/libexec/container/plugins/") {
		return true
	}

	// The Apple installer puts both executables in /usr/local/bin. Homebrew
	// can report a prefix symlink or the path within a versioned keg.
	name := filepath.Base(path)
	if name != "container-apiserver" && name != "container" {
		return false
	}
	switch filepath.Dir(path) {
	case "/usr/local/bin", "/opt/homebrew/bin":
		return true
	}
	if strings.Contains(path, "/opt/container/") {
		return true
	}
	_, kegPath, ok := strings.Cut(path, "/Cellar/container/")
	return ok && strings.Contains(kegPath, "/")
}
