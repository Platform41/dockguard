package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSettings(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings-store.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	return path
}

func TestSettingsContainStoragePathMatches(t *testing.T) {
	storage := t.TempDir()
	settings := writeSettings(t, `{"diskImageLocation": "`+storage+`"}`)

	if item := SettingsContainStoragePath(settings, storage); !item.OK {
		t.Fatalf("item = %#v, want OK", item)
	}
}

func TestSettingsContainStoragePathAcceptsUppercaseKey(t *testing.T) {
	storage := t.TempDir()
	settings := writeSettings(t, `{"DataFolder": "`+storage+`"}`)

	if item := SettingsContainStoragePath(settings, storage); !item.OK {
		t.Fatalf("item = %#v, want OK", item)
	}
}

func TestSettingsContainStoragePathFailsOnMismatch(t *testing.T) {
	settings := writeSettings(t, `{"diskImageLocation": "/Volumes/OtherDrive/DockerDesktop"}`)

	item := SettingsContainStoragePath(settings, t.TempDir())
	if item.OK || !strings.Contains(item.Message, "expected path not found") {
		t.Fatalf("item = %#v, want mismatch failure", item)
	}
}

func TestExtractSettingsPathsCollectsNestedKeys(t *testing.T) {
	content := []byte(`{
	"DiskImageLocation": "/Volumes/External/DockerDesktop",
	"nested": {
	  "DataFolder": "/Volumes/External/Data",
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

	for _, path := range []string{"/Volumes/External/DockerDesktop", "/Volumes/External/Data", "/Volumes/External/Storage"} {
		if !seen[path] {
			t.Fatalf("expected path %q in results, got %#v", path, paths)
		}
	}
}

func TestUnderDrive(t *testing.T) {
	drive := t.TempDir()
	inside := filepath.Join(drive, "DockerDesktop")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := drive + "-other"

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"inside", inside, true},
		{"the drive itself", drive, true},
		{"not yet created, inside", filepath.Join(drive, "later"), true},
		{"outside", t.TempDir(), false},
		{"shares a prefix only", sibling, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := UnderDrive("path on the drive", tt.path, drive)
			if item.OK != tt.want {
				t.Fatalf("UnderDrive(%q) OK = %v, want %v (%s)", tt.path, item.OK, tt.want, item.Message)
			}
		})
	}
}

func TestUnderDriveFollowsSymlinks(t *testing.T) {
	drive := t.TempDir()
	target := filepath.Join(drive, "DerivedData")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "DerivedData")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if item := UnderDrive("link on the drive", link, drive); !item.OK {
		t.Fatalf("item = %#v, want OK", item)
	}
}

func TestSkipAllMarksEveryItem(t *testing.T) {
	items := SkipAll([]Item{{Name: "a", OK: true}, {Name: "b", OK: false}})
	for _, item := range items {
		if !item.Skipped || item.OK || item.Message != "skipped: drive not mounted" {
			t.Fatalf("item = %#v, want skipped", item)
		}
	}
	if AllOK(items) {
		t.Fatal("AllOK(skipped) = true, want false")
	}
}

func TestDriveMounted(t *testing.T) {
	if !DriveMounted(t.TempDir()) {
		t.Fatal("DriveMounted(existing dir) = false")
	}
	if DriveMounted(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("DriveMounted(missing) = true")
	}
	if DriveMounted("") {
		t.Fatal("DriveMounted(\"\") = true")
	}
}

// Review on PR #4: "drive/link/../storage" where link points off the drive must fail. The
// filesystem follows the link before "..", so the real path is outside the drive.
func TestUnderDriveResolvesSymlinksBeforeParentTraversal(t *testing.T) {
	drive := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(outside, "storage"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "target"), filepath.Join(drive, "link")); err != nil {
		t.Fatal(err)
	}

	path := drive + "/link/../storage"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture: %s should reach %s/storage: %v", path, outside, err)
	}
	if item := UnderDrive("storage on the drive", path, drive); item.OK {
		t.Fatalf("UnderDrive(%q) OK = true, want false: %s", path, item.Message)
	}
}

func TestUnderDriveHandlesSymlinkLoops(t *testing.T) {
	drive := t.TempDir()
	a, b := filepath.Join(drive, "a"), filepath.Join(drive, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}

	// Must return rather than hang; the result doesn't matter beyond that.
	_ = UnderDrive("loop", filepath.Join(a, "x"), drive)
}

func TestUnderDriveFollowsRelativeSymlinks(t *testing.T) {
	drive := t.TempDir()
	if err := os.Mkdir(filepath.Join(drive, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", filepath.Join(drive, "rel")); err != nil {
		t.Fatal(err)
	}

	if item := UnderDrive("relative link", filepath.Join(drive, "rel", "x"), drive); !item.OK {
		t.Fatalf("item = %#v, want OK", item)
	}
}
