package containersystem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
)

func systemTarget() *Target {
	return New(config.Target{Name: "system", Kind: config.KindContainerSystem, ContainerSystem: &config.ContainerSystem{}}, "/Volumes/ExternalSSD", []string{"managed-app", "managed-db"})
}

func fakeSystem(t *testing.T, status, list string, code int) *[]string {
	t.Helper()
	old := run
	t.Cleanup(func() { run = old })
	calls := []string{}
	run = func(name string, args ...string) (string, int, error) {
		cmd := name + " " + strings.Join(args, " ")
		calls = append(calls, cmd)
		switch cmd {
		case "container system status --format json":
			return status, code, nil
		case "container list --all --format json":
			return list, 0, nil
		case "container system start", "container system stop":
			return "", 0, nil
		default:
			t.Fatalf("unexpected command %s", cmd)
			return "", 1, nil
		}
	}
	return &calls
}

// AS-106: state is shared with container stacks and fails closed for eject.
func TestSystemStates(t *testing.T) {
	for _, tt := range []struct {
		name, status, list string
		code               int
		want               target.State
	}{
		{"running", `{"status":"running"}`, `[]`, 0, target.Running},
		{"stopped", `{"status":"unregistered"}`, `bad`, 1, target.Stopped},
		{"unhealthy", `{"status":"unhealthy"}`, `[]`, 1, target.Unknown},
		{"bad list", `{"status":"running"}`, `null`, 0, target.Unknown},
		{"bad status", `no json`, `[]`, 0, target.Unknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fakeSystem(t, tt.status, tt.list, tt.code)
			s, err := systemTarget().State()
			if s != tt.want || (s == target.Unknown) != (err != nil) {
				t.Fatalf("state %s, %v", s, err)
			}
		})
	}
}

func TestStartConfirmsRunning(t *testing.T) {
	for _, status := range []string{`{"status":"running"}`, `{"status":"unregistered"}`, `garbled`} {
		calls := fakeSystem(t, status, `[]`, 0)
		err := systemTarget().Start()
		if (err == nil) != (strings.Contains(status, "running")) || (*calls)[0] != "container system start" {
			t.Fatalf("Start = %v; %v", err, *calls)
		}
	}
}

func TestSystemStopUnmanagedSafety(t *testing.T) {
	for _, tt := range []struct {
		name, list string
		refused    bool
	}{
		{"empty", `[]`, false},
		{"managed elsewhere", `[{"id":"managed-app","status":{"state":"running"}},{"id":"managed-db","status":{"state":"running"}}]`, false},
		{"unmanaged", `[{"id":"zeta","status":{"state":"running"}},{"id":"alpha","status":{"state":"starting"}}]`, true},
		{"unmanaged stopped", `[{"id":"other","status":{"state":"stopped"}}]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeSystem(t, `{"status":"running"}`, tt.list, 0)
			stopped, err := systemTarget().Stop()
			if tt.refused {
				if err == nil || stopped || !strings.Contains(err.Error(), "alpha, zeta") || strings.Contains(strings.Join(*calls, "|"), "container system stop") {
					t.Fatalf("Stop %v %v calls %v", stopped, err, *calls)
				}
			} else if err != nil || !stopped {
				t.Fatalf("Stop %v %v", stopped, err)
			}
		})
	}
}

func TestStopAlreadyStoppedOrUnknown(t *testing.T) {
	for _, status := range []string{`{"status":"unregistered"}`, `unreadable`} {
		calls := fakeSystem(t, status, `[]`, 1)
		stopped, err := systemTarget().Stop()
		if stopped || len(*calls) != 1 || (err == nil) != (strings.Contains(status, "unregistered")) {
			t.Fatalf("Stop %v %v calls %v", stopped, err, *calls)
		}
	}
}

func TestSystemCommandFailures(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		calls := fakeSystem(t, `{"status":"running"}`, `[]`, 0)
		probe := run
		run = func(n string, a ...string) (string, int, error) {
			if strings.Join(a, " ") == "system "+action {
				return "denied", 1, errors.New("failure")
			}
			return probe(n, a...)
		}
		var err error
		if action == "start" {
			err = systemTarget().Start()
		} else {
			_, err = systemTarget().Stop()
		}
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("%s: %v %v", action, err, *calls)
		}
	}
}

func TestSystemChecks(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	appRoot := filepath.Join(home, "Library", "Application Support", "com.apple.container")
	if err := os.MkdirAll(filepath.Dir(appRoot), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, appRoot); err != nil {
		t.Fatal(err)
	}
	oldLook, oldHome := lookPath, homeDir
	t.Cleanup(func() { lookPath, homeDir = oldLook, oldHome })
	lookPath = func(string) (string, error) { return "/bin/container", nil }
	homeDir = func() (string, error) { return home, nil }
	item := New(config.Target{Name: "system", Kind: config.KindContainerSystem, ContainerSystem: &config.ContainerSystem{DataPath: root}}, root, nil)
	if !checks.AllOK(item.Checks()) {
		t.Fatalf("checks: %+v", item.Checks())
	}
	lookPath = func(string) (string, error) { return "", errors.New("CLI missing") }
	if checks.AllOK(item.Checks()) {
		t.Fatal("missing CLI passed")
	}
	lookPath = func(string) (string, error) { return "/bin/container", nil }
	item.drive = t.TempDir()
	if checks.AllOK(item.Checks()) {
		t.Fatal("internal data passed")
	}
	homeDir = func() (string, error) { return "", errors.New("home unknown") }
	if checks.AllOK(item.Checks()) {
		t.Fatal("unknown home passed")
	}
}
