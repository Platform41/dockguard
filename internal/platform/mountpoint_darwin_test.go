package platform

import (
	"errors"
	"syscall"
	"testing"
)

func TestIsMountPointUsesFilesystemMountPath(t *testing.T) {
	orig := volumeStatfs
	defer func() { volumeStatfs = orig }()
	for _, tt := range []struct {
		path, mount string
		want        bool
	}{
		{"/Volumes/ExternalSSD", "/Volumes/ExternalSSD", true},
		{"/Volumes/ExternalSSD/folder", "/Volumes/ExternalSSD", false},
		{"/Volumes/ExternalSSD", "/", false},
		{"/", "/", true},
	} {
		volumeStatfs = func(_ string, st *syscall.Statfs_t) error {
			for i, ch := range []byte(tt.mount) {
				st.Mntonname[i] = int8(ch)
			}
			return nil
		}
		got, err := IsMountPoint(tt.path)
		if err != nil || got != tt.want {
			t.Fatalf("%s %s: %v %v", tt.path, tt.mount, got, err)
		}
	}
	volumeStatfs = func(string, *syscall.Statfs_t) error { return errors.New("denied") }
	if _, err := IsMountPoint("/Volumes/ExternalSSD"); err == nil {
		t.Fatal("ignored statfs failure")
	}
	volumeStatfs = func(string, *syscall.Statfs_t) error { return nil }
	if _, err := IsMountPoint("/Volumes/ExternalSSD"); err == nil {
		t.Fatal("accepted empty filesystem mount path")
	}
}
