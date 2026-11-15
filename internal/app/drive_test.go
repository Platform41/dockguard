package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/drive"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
)

const registeredUUID = "11111111-2222-3333-4444-555555555555"

func registeredSetup(t *testing.T, state drive.State, f *fake) *harness {
	t.Helper()
	h := setup(t, true, false, f)
	load := loadConfig
	loadConfig = func(p string) (config.Config, error) {
		cfg, err := load(p)
		cfg.DriveUUID = registeredUUID
		return cfg, err
	}
	oldInspect, oldAt, oldMount := inspectDrive, volumeAt, mountVolume
	t.Cleanup(func() { inspectDrive, volumeAt, mountVolume = oldInspect, oldAt, oldMount })
	inspectDrive = func(uuid, path string) drive.Status {
		return drive.Status{State: state, Message: string(state), Volume: platform.Volume{UUID: uuid, Name: "ExternalSSD", MountPoint: path}}
	}
	volumeAt = func(path string) (platform.Volume, error) {
		return platform.Volume{UUID: registeredUUID, Name: "ExternalSSD", MountPoint: path, Filesystem: "apfs"}, nil
	}
	mountVolume = func(context.Context, string) error { t.Fatal("unexpected mount"); return nil }
	return h
}

// AS-63–67: invalid identity skips target checks/probes, refuses start and eject.
func TestRegisteredIdentityFailClosed(t *testing.T) {
	for _, state := range []drive.State{drive.NotMounted, drive.NotConnected, drive.WrongVolume, drive.StrayFolder, drive.MountedElsewhere, drive.Unknown} {
		for _, command := range []string{"start", "check", "list", "status"} {
			t.Run(string(state)+"/"+command, func(t *testing.T) {
				f := &fake{name: "docker", noChecks: true, noProbing: true}
				h := registeredSetup(t, state, f)
				code, err := Run([]string{command})
				if command != "list" && code != 1 {
					t.Fatalf("code %d error %v", code, err)
				}
				if !strings.Contains(h.out.String(), string(state)) {
					t.Fatalf("output %s", h.out)
				}
				if f.started {
					t.Fatal("started with invalid identity")
				}
			})
		}
		t.Run(string(state)+"/eject", func(t *testing.T) {
			f := &fake{name: "docker", states: states(target.Stopped)}
			h := registeredSetup(t, state, f)
			code, _ := Run([]string{"stop", "--all", "--eject"})
			if code != 1 || h.ejected {
				t.Fatal("ejected unverified drive")
			}
		})
	}
}

// AS-60/61: registration is read-only and requires an actual mount point.
func TestDriveRegistration(t *testing.T) {
	h := setup(t, true, false, &fake{name: "docker"})
	old := volumeAt
	oldMountPoint := isMountPoint
	defer func() { volumeAt = old; isMountPoint = oldMountPoint }()
	isMountPoint = func(string) (bool, error) { return true, nil }
	cfg, _ := loadConfig("")
	volumeAt = func(string) (platform.Volume, error) {
		return platform.Volume{UUID: registeredUUID, Name: "ExternalSSD", MountPoint: cfg.Drive, Filesystem: "apfs"}, nil
	}
	code, err := Run([]string{"drive"})
	if code != 0 || err != nil || !strings.Contains(h.out.String(), "drive_uuid: "+registeredUUID) {
		t.Fatalf("%d %v %s", code, err, h.out)
	}
	volumeAt = func(string) (platform.Volume, error) {
		return platform.Volume{UUID: registeredUUID, MountPoint: "/"}, nil
	}
	if code, _ := Run([]string{"drive"}); code != 1 {
		t.Fatal("registered directory on enclosing volume")
	}
}

func TestRegistrationRejectsOrdinaryFolderBeforeDiskutil(t *testing.T) {
	setup(t, true, false, &fake{name: "docker"})
	oldAt, oldMountPoint := volumeAt, isMountPoint
	defer func() { volumeAt, isMountPoint = oldAt, oldMountPoint }()
	isMountPoint = func(string) (bool, error) { return false, nil }
	volumeAt = func(string) (platform.Volume, error) {
		t.Fatal("must not inspect non-mount point with diskutil")
		return platform.Volume{}, nil
	}
	code, err := Run([]string{"drive"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "not a mount point") {
		t.Fatalf("%d %v", code, err)
	}
}

func TestLegacyHintExplainsMigration(t *testing.T) {
	h := setup(t, true, true, &fake{name: "docker", checksOK: true, states: states(target.Stopped)})
	Run([]string{"list"})
	if !strings.Contains(h.out.String(), "current config format") || strings.Contains(h.out.String(), "Drive not registered; run dockguard drive.") {
		t.Fatal(h.out)
	}
}

// AS-68–73, AS-102/103: actions re-inspect even on timeout/failure.
func TestMountCommand(t *testing.T) {
	for _, outcome := range []struct {
		name  string
		err   error
		after drive.State
		want  int
	}{
		{"success", nil, drive.Mounted, 0}, {"elsewhere", nil, drive.MountedElsewhere, 1}, {"timeout", context.DeadlineExceeded, drive.NotMounted, 1}, {"late mount", context.DeadlineExceeded, drive.Mounted, 1}, {"failed", errors.New("busy"), drive.NotMounted, 1},
	} {
		t.Run(outcome.name, func(t *testing.T) {
			h := registeredSetup(t, drive.NotMounted, &fake{name: "docker"})
			calls := 0
			inspectDrive = func(string, string) drive.Status {
				calls++
				state := drive.NotMounted
				if calls > 1 {
					state = outcome.after
				}
				return drive.Status{State: state, Message: string(state), Volume: platform.Volume{Name: "ExternalSSD"}}
			}
			mounts := 0
			mountVolume = func(context.Context, string) error { mounts++; return outcome.err }
			code, err := Run([]string{"mount"})
			if code != outcome.want || calls != 2 || mounts != 1 {
				t.Fatalf("code %d err %v calls %d mounts %d", code, err, calls, mounts)
			}
			if !strings.Contains(h.out.String(), "Mounting ExternalSSD") {
				t.Fatal(h.out)
			}
		})
	}
	for _, state := range []drive.State{drive.Mounted, drive.NotConnected, drive.WrongVolume, drive.StrayFolder, drive.MountedElsewhere, drive.Unknown} {
		t.Run(string(state), func(t *testing.T) {
			registeredSetup(t, state, &fake{name: "docker"})
			code, _ := Run([]string{"mount"})
			if (code == 0) != (state == drive.Mounted) {
				t.Fatalf("code %d", code)
			}
		})
	}
	t.Run("unregistered", func(t *testing.T) {
		setup(t, true, false, &fake{name: "docker"})
		if code, _ := Run([]string{"mount"}); code != 1 {
			t.Fatal(code)
		}
	})
}

// AS-62/74: hints appear only for unregistered list/status; mounted identity passes.
func TestRegisteredMountedAndUnregisteredHint(t *testing.T) {
	for _, command := range []string{"list", "status", "check"} {
		t.Run(command, func(t *testing.T) {
			f := &fake{name: "docker", checksOK: true, states: states(target.Stopped)}
			h := registeredSetup(t, drive.Mounted, f)
			if code, err := Run([]string{command}); code != 0 || err != nil {
				t.Fatalf("%d %v", code, err)
			}
			if strings.Contains(h.out.String(), "Drive not registered") {
				t.Fatal(h.out)
			}
		})
	}
	t.Run("hint", func(t *testing.T) {
		h := setup(t, true, false, &fake{name: "docker", checksOK: true, states: states(target.Stopped)})
		Run([]string{"list"})
		if strings.Count(h.out.String(), "Drive not registered; run dockguard drive.") != 1 {
			t.Fatal(h.out)
		}
	})
}

// AS-76/77: snapshot gating and controller action share CLI semantics.
func TestDashboardRegisteredDrive(t *testing.T) {
	registeredSetup(t, drive.NotMounted, &fake{name: "docker", noProbing: true})
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	ctl := &controller{cfg: cfg, entries: entries}
	snap := ctl.Snapshot()
	if !snap.CanMount || snap.Mounted || snap.DriveStatus != "not mounted" {
		t.Fatalf("snapshot %+v", snap)
	}
	mountVolume = func(context.Context, string) error { return errors.New("busy") }
	ok, text := ctl.Run(ui.Mount, "")
	if ok || !strings.Contains(text, "busy") {
		t.Fatalf("%v %s", ok, text)
	}
}

// AS-67: identity is checked again after target/holder probes before eject.
func TestEjectRechecksRegisteredIdentity(t *testing.T) {
	h := registeredSetup(t, drive.Mounted, &fake{name: "docker", states: states(target.Stopped)})
	calls := 0
	inspectDrive = func(string, string) drive.Status {
		calls++
		state := drive.Mounted
		if calls > 1 {
			state = drive.WrongVolume
		}
		return drive.Status{State: state, Message: string(state)}
	}
	code, _ := Run([]string{"stop", "--all", "--eject"})
	if code != 1 || h.ejected || calls < 2 {
		t.Fatalf("code %d ejected %v calls %d", code, h.ejected, calls)
	}
}
