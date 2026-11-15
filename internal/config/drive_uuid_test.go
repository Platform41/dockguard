package config

import (
	"strings"
	"testing"
)

// AS-75: optional identity accepts hexadecimal UUIDs and rejects malformed keys.
func TestDriveUUID(t *testing.T) {
	const base = "drive: /Volumes/ExternalSSD\ntargets:\n  editor:\n    kind: app\n    app: Editor\n"
	for _, uuid := range []string{"abcdefab-1234-5678-9012-abcdefabcdef", "ABCDEFAB-1234-5678-9012-ABCDEFABCDEF"} {
		cfg, err := Parse([]byte(base + "drive_uuid: " + uuid + "\n"))
		if err != nil || cfg.DriveUUID != uuid {
			t.Fatalf("cfg %+v, error %v", cfg, err)
		}
	}
	for _, value := range []string{"''", "null", "not-a-uuid", "GGGGGGGG-1234-5678-9012-ABCDEFABCDEF", "1234", "[]"} {
		_, err := Parse([]byte(base + "drive_uuid: " + value + "\n"))
		if err == nil || !strings.Contains(err.Error(), "drive_uuid") {
			t.Fatalf("value %s: %v", value, err)
		}
	}
	if _, err := Parse([]byte("external_mount_path: /Volumes/ExternalSSD\ndrive_uuid: abcdefab-1234-5678-9012-abcdefabcdef\n")); err == nil {
		t.Fatal("legacy accepted identity")
	}
	cfg, err := Parse([]byte(base))
	if err != nil || cfg.DriveUUID != "" {
		t.Fatalf("optional key %+v: %v", cfg, err)
	}
}
