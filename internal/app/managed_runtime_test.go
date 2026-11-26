package app

import (
	"strings"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/holders"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
)

func managedRuntimeHolders() {
	list := []platform.Holder{
		{Name: "container-runtime-linux", PID: 42, UID: 501},
		{Name: "com.apple.Virtualization.VirtualMachine", PID: 43, UID: 501},
	}
	paths := []string{
		"/opt/homebrew/Cellar/container/1.7.1/libexec/container-plugins/container-runtime-linux/bin/container-runtime-linux",
		"/System/Library/Frameworks/Virtualization.framework/Versions/A/XPCServices/com.apple.Virtualization.VirtualMachine.xpc/Contents/MacOS/com.apple.Virtualization.VirtualMachine",
	}
	holdersOf = func(string) ([]platform.Holder, error) { return list, nil }
	factory := holderService
	holderService = func() holders.Service {
		s := factory()
		s.Inspect = func(pid int) (platform.Process, error) {
			i := pid - list[0].PID
			return platform.Process{Name: list[i].Name, UID: list[i].UID, Path: paths[i], Started: time.Unix(10, 1)}, nil
		}
		return s
	}
}

func assertManagedRuntimeOutput(t *testing.T, out string) {
	t.Helper()
	for _, message := range []string{
		"container-runtime-linux (PID 42) [managed runtime] — stop it through its DockGuard target or container stop",
		"com.apple.Virtualization.VirtualMachine (PID 43) [managed runtime] — stop it through its DockGuard target or container stop",
	} {
		if !strings.Contains(out, message) {
			t.Errorf("missing %q in %s", message, out)
		}
	}
	for _, question := range []string{"Quit it?", "Force-quit it?", "kill 42", "kill 43"} {
		if strings.Contains(out, question) {
			t.Errorf("unexpected stop offer %q in %s", question, out)
		}
	}
}

func TestManagedRuntimeHoldersCLIAS159(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		terminal bool
	}{
		{"list", []string{"holders"}, true},
		{"stop", []string{"holders", "--stop"}, true},
		{"stop without terminal", []string{"holders", "--stop"}, false},
		{"eject", []string{"stop", "myapp", "--eject"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t, true, false, &fake{name: "myapp", states: states(target.Stopped)})
			holderCLI(t, tt.terminal, "yes\nyes\n")
			managedRuntimeHolders()
			code, err := Run(tt.args)
			if code != 1 || h.ejected {
				t.Fatalf("Run = %d, %v; ejected = %v; output = %s", code, err, h.ejected, h.out)
			}
			assertManagedRuntimeOutput(t, h.out.String())
		})
	}
}

func TestDashboardProcessActionLeavesManagedRuntimeAS159(t *testing.T) {
	h, c := dashboardHolderSetup(t)
	managedRuntimeHolders()
	snapshot := c.Snapshot()
	for _, blocker := range ui.Blockers(snapshot) {
		if !strings.Contains(blocker, "[managed runtime] — stop it through its DockGuard target or container stop") {
			t.Errorf("unexpected eject blocker %q", blocker)
		}
	}
	if got := len(snapshot.Holders); got != 2 {
		t.Fatalf("holders = %d, want 2", got)
	}

	sequence, _ := startDashboardStop(t, c, ui.StopProcesses, "", nil)
	update := sequence.Next()
	if update.Question != nil || update.OK || h.ejected {
		t.Fatalf("stop update = %+v; ejected = %v", update, h.ejected)
	}
	assertManagedRuntimeOutput(t, update.Output)
}
