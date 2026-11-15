package container

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

type volumeInfo struct {
	blocks int64
	mode   os.FileMode
	sys    any
}

func (v volumeInfo) Name() string       { return "volume.img" }
func (v volumeInfo) Size() int64        { return 512 * 1024 * 1024 * 1024 }
func (v volumeInfo) Mode() os.FileMode  { return v.mode }
func (v volumeInfo) ModTime() time.Time { return time.Time{} }
func (v volumeInfo) IsDir() bool        { return v.mode.IsDir() }
func (v volumeInfo) Sys() any {
	if v.sys != nil {
		return v.sys
	}
	return &syscall.Stat_t{Blocks: v.blocks}
}

// AS-79/80/82/83: metadata is available without any container CLI invocation.
func TestVolumesStatOnly(t *testing.T) {
	m := &machine{}
	install(t, m)
	old := volumeStat
	defer func() { volumeStat = old }()
	for _, tt := range []struct {
		name  string
		info  os.FileInfo
		err   error
		state string
	}{
		{"present", volumeInfo{blocks: 524288}, nil, "present"},
		{"missing", nil, os.ErrNotExist, "missing"},
		{"permission", nil, os.ErrPermission, "unknown"},
		{"directory", volumeInfo{mode: os.ModeDir}, nil, "unknown"},
		{"metadata missing", volumeInfo{sys: struct{}{}}, nil, "unknown"},
		{"negative blocks", volumeInfo{blocks: -1}, nil, "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			volumeStat = func(path string) (os.FileInfo, error) {
				calls++
				if path != "/data/volumes/db/volume.img" {
					t.Fatalf("path %s", path)
				}
				return tt.info, tt.err
			}
			target := fizzy(false)
			target.cfg.Container.DataPath = "/data"
			target.cfg.Container.Volumes = []string{"db"}
			got := target.Volumes()
			if len(got) != 1 || got[0].State != tt.state || got[0].Path != "/data/volumes/db" || calls != 1 {
				t.Fatalf("volumes %+v calls %d", got, calls)
			}
			if tt.state == "present" && got[0].AllocatedBytes != 268435456 {
				t.Fatalf("used %d", got[0].AllocatedBytes)
			}
			if errors.Is(tt.err, os.ErrPermission) && got[0].Err == nil {
				t.Fatal("lost stat error")
			}
			if len(m.ran) != 0 {
				t.Fatalf("container CLI called: %v", m.ran)
			}
		})
	}
}

func TestVolumesPreserveOrderAndDataPathSemantics(t *testing.T) {
	old := volumeStat
	defer func() { volumeStat = old }()
	var paths []string
	volumeStat = func(path string) (os.FileInfo, error) { paths = append(paths, path); return nil, os.ErrNotExist }
	target := fizzy(false)
	target.cfg.Container.DataPath = "/data/alias/.."
	target.cfg.Container.Volumes = []string{"z", "a"}
	got := target.Volumes()
	if got[0].Name != "z" || got[1].Name != "a" || paths[0] != "/data/alias/../volumes/z/volume.img" {
		t.Fatalf("%+v %v", got, paths)
	}
}
