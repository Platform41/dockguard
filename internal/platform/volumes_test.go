package platform

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVolumeCommandErrorMessage(t *testing.T) {
	orig := volumeRun
	defer func() { volumeRun = orig }()
	for _, tt := range []struct{ out, want string }{
		{`<?xml version="1.0"?><plist><dict><key>Error</key><true/><key>ErrorMessage</key><string>Could not find disk: /Volumes/ExternalSSD</string></dict></plist>`, "Could not find disk: /Volumes/ExternalSSD"},
		{" busy \n", "busy"},
		{"<malformed>", "<malformed>"},
	} {
		volumeRun = func(context.Context, string, ...string) (string, int, error) { return tt.out, 1, nil }
		_, err := VolumeAt("/Volumes/ExternalSSD")
		if err == nil || err.Error() != "diskutil info exited 1: "+tt.want {
			t.Fatalf("error %v", err)
		}
		if strings.Contains(tt.out, "ErrorMessage") && strings.Contains(err.Error(), "<?xml") {
			t.Fatal(err)
		}
	}
}

const testVolumeUUID = "11111111-2222-3333-4444-555555555555"
const volumePlist = `<plist version="1.0"><dict><key>VolumeUUID</key><string>` + testVolumeUUID + `</string><key>VolumeName</key><string>ExternalSSD</string><key>MountPoint</key><string>/Volumes/ExternalSSD</string><key>FilesystemType</key><string>apfs</string><key>Internal</key><false/></dict></plist>`
const listPlist = `<plist version="1.0"><dict><key>AllDisksAndPartitions</key><array><dict><key>DeviceIdentifier</key><string>disk9</string><key>APFSVolumes</key><array><dict><key>VolumeUUID</key><string>` + testVolumeUUID + `</string></dict></array></dict></array></dict></plist>`

// AS-100/101: only a valid inventory can establish absence.
func TestFindVolumeInventory(t *testing.T) {
	for _, tt := range []struct {
		name, listing          string
		code                   int
		wantMissing, wantError bool
	}{
		{"present", listPlist, 0, false, false},
		{"absent", `<plist><dict><key>AllDisksAndPartitions</key><array/></dict></plist>`, 0, true, true},
		{"malformed", `<plist><dict>`, 0, false, true},
		{"missing schema", `<plist><dict/></plist>`, 0, false, true},
		{"invalid disk", `<plist><dict><key>AllDisksAndPartitions</key><array><string>broken</string></array></dict></plist>`, 0, false, true},
		{"invalid UUID type", `<plist><dict><key>AllDisksAndPartitions</key><array><dict><key>VolumeUUID</key><integer>42</integer></dict></array></dict></plist>`, 0, false, true},
		{"failed listing", "Could not find disk", 1, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			original := volumeRun
			defer func() { volumeRun = original }()
			calls := 0
			volumeRun = func(ctx context.Context, name string, args ...string) (string, int, error) {
				calls++
				if name != "diskutil" {
					t.Fatalf("command %s", name)
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("missing deadline")
				}
				if calls == 1 {
					if !reflect.DeepEqual(args, []string{"list", "-plist"}) {
						t.Fatalf("args %v", args)
					}
					return tt.listing, tt.code, nil
				}
				return volumePlist, 0, nil
			}
			got, err := FindVolume(testVolumeUUID)
			if (err != nil) != tt.wantError || errors.Is(err, ErrVolumeNotFound) != tt.wantMissing {
				t.Fatalf("error %v", err)
			}
			if tt.wantError && calls != 1 {
				t.Fatalf("calls %d", calls)
			}
			if !tt.wantError && (got.UUID != testVolumeUUID || got.MountPoint != "/Volumes/ExternalSSD" || got.Internal) {
				t.Fatalf("volume %+v", got)
			}
		})
	}
}

func TestFindVolumeInfoFailureIsUnknown(t *testing.T) {
	original := volumeRun
	defer func() { volumeRun = original }()
	calls := 0
	volumeRun = func(context.Context, string, ...string) (string, int, error) {
		calls++
		if calls == 1 {
			return listPlist, 0, nil
		}
		return "Could not find disk", 1, nil
	}
	_, err := FindVolume(testVolumeUUID)
	if err == nil || errors.Is(err, ErrVolumeNotFound) {
		t.Fatalf("error %v", err)
	}
}

func TestVolumeAtRequiresIdentity(t *testing.T) {
	original := volumeRun
	defer func() { volumeRun = original }()
	for _, data := range []string{`<plist><dict/></plist>`, `<plist><dict><key>VolumeUUID</key><integer>42</integer></dict></plist>`, volumePlist + `garbage`} {
		volumeRun = func(context.Context, string, ...string) (string, int, error) { return data, 0, nil }
		if _, err := VolumeAt("/Volumes/ExternalSSD"); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

// AS-102/103: deadline and exit failures are distinct; no real mount is run.
func TestMountVolumeBoundAndFailures(t *testing.T) {
	original := volumeRun
	defer func() { volumeRun = original }()
	volumeRun = func(ctx context.Context, name string, args ...string) (string, int, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 90*time.Second {
			t.Fatal("missing mount bound")
		}
		if !reflect.DeepEqual(args, []string{"mount", testVolumeUUID}) {
			t.Fatalf("args %v", args)
		}
		return "busy", 1, nil
	}
	if err := MountVolume(context.Background(), testVolumeUUID); err == nil {
		t.Fatal("ignored exit failure")
	}
	volumeRun = func(ctx context.Context, _ string, _ ...string) (string, int, error) {
		<-ctx.Done()
		return "", -1, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := MountVolume(ctx, testVolumeUUID); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := MountVolume(ctx, testVolumeUUID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v", err)
	}
}
