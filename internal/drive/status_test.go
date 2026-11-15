package drive

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/platform41/dockguard/internal/platform"
)

// AS-62 through AS-66, AS-100/101: fail closed for all unverified states.
func TestStatus(t *testing.T) {
	const path = "/Volumes/ExternalSSD"
	const uuid = "11111111-2222-3333-4444-555555555555"
	for _, tt := range []struct {
		name, mount, atUUID, atMount string
		findErr, statErr, atErr      error
		want                         State
	}{
		{name: "mounted", mount: path, want: Mounted},
		{name: "attached unmounted", statErr: fs.ErrNotExist, want: NotMounted},
		{name: "not connected", findErr: platform.ErrVolumeNotFound, want: NotConnected},
		{name: "mounted elsewhere", mount: path + " 1", want: MountedElsewhere},
		{name: "wrong volume", atUUID: "other-uuid", atMount: path, want: WrongVolume},
		{name: "stray folder", atUUID: "internal-uuid", atMount: "/", want: StrayFolder},
		{name: "inventory failure", findErr: errors.New("diskutil failed"), want: Unknown},
		{name: "path permission", statErr: fs.ErrPermission, want: Unknown},
		{name: "path inspection failure", atErr: errors.New("timeout"), want: Unknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			atCalls := 0
			inspector := Inspector{
				FindVolume: func(string) (platform.Volume, error) {
					return platform.Volume{UUID: uuid, Name: "ExternalSSD", MountPoint: tt.mount}, tt.findErr
				},
				VolumeAt: func(string) (platform.Volume, error) {
					atCalls++
					return platform.Volume{UUID: tt.atUUID, MountPoint: tt.atMount}, tt.atErr
				},
				PathExists: func(string) error { return tt.statErr },
			}
			got := inspector.Status(uuid, path)
			if got.State != tt.want || got.Mounted() != (tt.want == Mounted) {
				t.Fatalf("status %+v", got)
			}
			if got.Message == "" {
				t.Fatal("missing explanation")
			}
			if tt.findErr != nil && atCalls != 0 {
				t.Fatal("queried path after failed discovery")
			}
		})
	}
}

func TestStatusRejectsWrongReturnedIdentity(t *testing.T) {
	inspector := Inspector{FindVolume: func(string) (platform.Volume, error) {
		return platform.Volume{UUID: "another", MountPoint: "/Volumes/ExternalSSD"}, nil
	}}
	if got := inspector.Status("registered", "/Volumes/ExternalSSD"); got.State != Unknown {
		t.Fatalf("status %+v", got)
	}
}
