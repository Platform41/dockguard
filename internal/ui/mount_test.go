package ui

import (
	"strings"
	"testing"
)

// AS-76/77: mount needs no confirmation, runs through the existing busy queue.
func TestMountKeyAndHelp(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		f := &fakeCtl{snap: Snapshot{Drive: "/Volumes/ExternalSSD", CanMount: enabled, DriveStatus: "not mounted", DriveMessage: "attached but unmounted"}}
		m := loaded(f)
		view := m.View()
		if strings.Contains(view, "m mount") != enabled {
			t.Fatalf("enabled %v: %s", enabled, view)
		}
		m, cmd := press(m, "m")
		if (cmd != nil) != enabled || m.confirming != nil {
			t.Fatalf("enabled %v cmd %v", enabled, cmd)
		}
		if enabled {
			msg := cmd()
			if len(f.calls) != 1 || f.calls[0].action != Mount {
				t.Fatal(f.calls)
			}
			next, _ := m.Update(msg)
			if !next.(Model).refreshing {
				t.Fatal("no refresh")
			}
		}
	}
}

func TestMountWaitsForProbe(t *testing.T) {
	f := &fakeCtl{snap: Snapshot{CanMount: true}}
	m := loaded(f)
	m.refreshing = true
	m, cmd := press(m, "m")
	if cmd != nil || m.queued == nil {
		t.Fatal("mount overlapped probe")
	}
}
