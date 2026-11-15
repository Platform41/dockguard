package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/drive"
	"github.com/platform41/dockguard/internal/target"
	containerkind "github.com/platform41/dockguard/internal/target/container"
)

type volumeFake struct {
	*fake
	volumes []containerkind.VolumeStatus
	lookups int
}

func (v *volumeFake) Volumes() []containerkind.VolumeStatus { v.lookups++; return v.volumes }

func volumeSetup(t *testing.T, mounted bool) (*harness, *volumeFake) {
	t.Helper()
	f := &fake{name: "fizzy", kind: "container", checksOK: true, states: states(target.Stopped)}
	h := setup(t, mounted, false, f)
	cfg, _ := loadConfig("")
	cfg.Targets[0].Container = &config.Container{DataPath: "/data", Volumes: []string{"db", "backup"}}
	v := &volumeFake{fake: f, volumes: []containerkind.VolumeStatus{{Name: "db", Path: "/data/volumes/db", State: "present", AllocatedBytes: 268435456}, {Name: "backup", Path: "/data/volumes/backup", State: "missing"}}}
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return []entry{{t: v, cfg: cfg.Targets[0]}}, nil }
	return h, v
}

// AS-78–81: subrows follow the stack in config order and never affect list's exit.
func TestListVolumeRows(t *testing.T) {
	for _, state := range []string{"present", "missing", "unknown", "drive not mounted"} {
		t.Run(state, func(t *testing.T) {
			h, v := volumeSetup(t, state != "drive not mounted")
			v.volumes[0].State = state
			if state == "unknown" {
				v.volumes[0].Err = errors.New("denied")
			}
			code, err := Run([]string{"list"})
			if code != 0 || err != nil {
				t.Fatalf("%d %v", code, err)
			}
			label := state
			if state == "present" {
				label = "present, 268 MB"
			}
			if state == "unknown" {
				label = "unknown (denied)"
			}
			text := h.out.String()
			db := strings.Index(text, "  └ db")
			backup := strings.Index(text, "  └ backup")
			if db < strings.Index(text, "fizzy") || db < 0 || backup < db || !strings.Contains(text, label) || !strings.Contains(text, "/data/volumes/db") {
				t.Fatal(text)
			}
			if state == "drive not mounted" && v.lookups != 0 {
				t.Fatal("lookup while unmounted")
			}
		})
	}
}

// AS-84: disk information follows checks, even if the file is missing/unknown.
func TestStatusVolumesAreInformational(t *testing.T) {
	for _, state := range []string{"present", "missing", "unknown"} {
		t.Run(state, func(t *testing.T) {
			h, v := volumeSetup(t, true)
			v.volumes[0].State = state
			if state == "unknown" {
				v.volumes[0].Err = errors.New("denied")
			}
			code, err := Run([]string{"status"})
			if code != 0 || err != nil {
				t.Fatalf("%d %v: %s", code, err, h.out)
			}
			text := h.out.String()
			if strings.Index(text, "  volume db:") < strings.Index(text, "fizzy check") || !strings.Contains(text, "  volume backup: missing (/data/volumes/backup)\n") || !strings.HasSuffix(text, "Status: ready\n") {
				t.Fatal(text)
			}
		})
	}
}

func TestVolumesDoNotMaskFailingChecks(t *testing.T) {
	h, v := volumeSetup(t, true)
	v.fake.checksOK = false
	code, err := Run([]string{"status"})
	if code != 1 || err != nil || !strings.HasSuffix(h.out.String(), "Status: not ready\n") {
		t.Fatalf("%d %v %s", code, err, h.out)
	}
}

// AS-81 plus D-021: a wrong registered volume suppresses stats, not just a missing directory.
func TestVolumesRespectVerifiedDriveIdentity(t *testing.T) {
	h, v := volumeSetup(t, true)
	load := loadConfig
	loadConfig = func(p string) (config.Config, error) {
		cfg, err := load(p)
		cfg.DriveUUID = registeredUUID
		return cfg, err
	}
	old := inspectDrive
	defer func() { inspectDrive = old }()
	inspectDrive = func(string, string) drive.Status {
		return drive.Status{State: drive.WrongVolume, Message: "wrong volume"}
	}
	for _, command := range []string{"list", "status"} {
		h.out.Reset()
		Run([]string{command})
		if !strings.Contains(h.out.String(), "drive not mounted") || v.lookups != 0 {
			t.Fatalf("%s lookups %d", h.out, v.lookups)
		}
	}
}

// AS-86: exact output from the merged baseline is retained without configured volumes.
func TestNoVolumesOutputUnchanged(t *testing.T) {
	h, v := volumeSetup(t, true)
	load := loadConfig
	loadConfig = func(p string) (config.Config, error) {
		cfg, err := load(p)
		cfg.Targets[0].Container.Volumes = nil
		return cfg, err
	}
	buildTargets = func(cfg config.Config) ([]entry, error) { return []entry{{t: v, cfg: cfg.Targets[0]}}, nil }
	cfg, _ := loadConfig("")
	header := fmt.Sprintf("Config: %s\nDrive:  %s (mounted)\nDrive not registered; run dockguard drive.\n", cfg.Path, cfg.Drive)
	for _, tt := range []struct{ command, want string }{
		{"list", header + "\nNAME   KIND       STATE    PATH\nfizzy  container  stopped  /Volumes/X/fizzy\n"},
		{"status", header + "\nfizzy (container): stopped\n- [ok] external mount path configured (" + cfg.Drive + ")\n- [ok] external mount path exists (directory present)\n- [ok] fizzy check\n\nStatus: ready\n"},
	} {
		h.out.Reset()
		Run([]string{tt.command})
		if h.out.String() != tt.want {
			t.Fatalf("%s actual %q\nwant %q", tt.command, h.out.String(), tt.want)
		}
	}
	if v.lookups != 0 {
		t.Fatal("queried unconfigured volumes")
	}
}

// AS-87: volumes remain neither lifecycle targets nor dashboard rows.
func TestVolumeNamesNotTargetsAndNoLifecycleLookup(t *testing.T) {
	h, v := volumeSetup(t, true)
	for _, command := range []string{"start", "stop", "check"} {
		if code, err := Run([]string{command, "db"}); code != 1 || err == nil || !strings.Contains(err.Error(), "unknown target") {
			t.Fatalf("%s %d %v", command, code, err)
		}
	}
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	snap := (&controller{cfg: cfg, entries: entries}).Snapshot()
	if len(snap.Rows) != 1 || snap.Rows[0].Name != "fizzy" {
		t.Fatalf("snapshot %+v", snap)
	}
	for _, args := range [][]string{{"start", "fizzy"}, {"check", "fizzy"}, {"stop", "fizzy"}, {"stop", "--all", "--eject"}} {
		h.out.Reset()
		code, err := Run(args)
		withVolumes := h.out.String()
		cfg.Targets[0].Container.Volumes = nil
		h.out.Reset()
		withoutCode, withoutErr := Run(args)
		if code != withoutCode || fmt.Sprint(err) != fmt.Sprint(withoutErr) || withVolumes != h.out.String() {
			t.Fatalf("volume config changed %v output", args)
		}
		cfg.Targets[0].Container.Volumes = []string{"db", "backup"}
	}
	if v.lookups != 0 {
		t.Fatalf("lifecycle queried volumes %d", v.lookups)
	}
}
