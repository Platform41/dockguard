// Package pathutil resolves paths the way the filesystem does, for checks that must not be fooled
// by symlinks.
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// maxSymlinks bounds symlink chains, as the kernel does, so a loop can't hang a check.
const maxSymlinks = 40

// Resolve returns the real path the filesystem would reach. It walks the path one part at a time
// and follows each symlink before applying a later "..", so "drive/link/../x" resolves through
// the link's target. Cleaning first would drop "link/.." and wrongly stay on the drive.
//
// Once a part doesn't exist, the rest is joined as written, so a path that isn't created yet
// still resolves through its existing parents (/var/x and /private/var/x compare equal on macOS).
func Resolve(path string) string {
	if !filepath.IsAbs(path) {
		if wd, err := os.Getwd(); err == nil {
			path = wd + string(filepath.Separator) + path
		}
	}
	return walk(path, 0)
}

func walk(path string, depth int) string {
	parts := strings.Split(path, string(filepath.Separator))
	current := string(filepath.Separator)

	for i, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			current = filepath.Dir(current)
			continue
		}

		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if err != nil {
			return filepath.Join(current, strings.Join(parts[i:], string(filepath.Separator)))
		}

		if info.Mode()&os.ModeSymlink == 0 {
			current = next
			continue
		}

		link, err := os.Readlink(next)
		if err != nil || depth >= maxSymlinks {
			return filepath.Join(current, strings.Join(parts[i:], string(filepath.Separator)))
		}
		if !filepath.IsAbs(link) {
			link = current + string(filepath.Separator) + link
		}
		current = walk(link, depth+1)
	}

	return current
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Within reports whether path is dir or inside it. Both should already be resolved.
func Within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
