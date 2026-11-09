package container

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
)

// machine fakes the container CLI: the system is up or down, and containers have states.
type machine struct {
	systemUp   bool
	statusErr  error  // the CLI couldn't run
	statusOut  string // overrides the status output, with statusCode
	statusCode int
	listJSON   string // overrides the generated list
	states     map[string]string
	ran        []string
	downStops  []string // containers the down script stops
	scriptCode int
}

func install(t *testing.T, m *machine) {
	t.Helper()
	origRun, origDetached, origLook, origHome := run, runDetached, lookPath, homeDir
	t.Cleanup(func() { run, runDetached, lookPath, homeDir = origRun, origDetached, origLook, origHome })

	run = func(name string, args ...string) (string, int, error) {
		cmd := name + " " + strings.Join(args, " ")
		m.ran = append(m.ran, cmd)
		switch cmd {
		case "container system status --format json":
			switch {
			case m.statusErr != nil:
				return "", -1, m.statusErr
			case m.statusOut != "":
				return m.statusOut, m.statusCode, nil
			case !m.systemUp:
				return `{"status":"unregistered"}`, 1, nil
			}
			return `{"resources":{"containersRunning":0},"status":"running"}`, 0, nil
		case "container list --all --format json":
			if m.listJSON != "" {
				return m.listJSON, 0, nil
			}
			var list []map[string]any
			for id, state := range m.states {
				list = append(list, map[string]any{"id": id, "status": map[string]any{"state": state, "networks": []any{}}})
			}
			out, _ := json.Marshal(list)
			return string(out), 0, nil
		case "container system stop":
			m.systemUp = false
			return "", 0, nil
		}
		return "", 1, nil
	}
	runDetached = func(name string, args ...string) (string, int, error) {
		m.ran = append(m.ran, name)
		if strings.HasSuffix(name, "down.sh") {
			for _, id := range m.downStops {
				m.states[id] = "stopped"
			}
		}
		return "", m.scriptCode, nil
	}
	lookPath = func(string) (string, error) { return "/opt/homebrew/bin/container", nil }
}

func fizzy(stopSystem bool) *Target {
	return New(config.Target{Name: "fizzy", Kind: config.KindContainer, FailIfAlreadyRunning: true,
		Container: &config.Container{Up: "/x/fizzy-up.sh", Down: "/x/fizzy-down.sh",
			Containers: []string{"fizzy-db", "fizzy-app"}, StopSystem: stopSystem}}, "/Volumes/X")
}

func ran(m *machine, cmd string) bool {
	for _, c := range m.ran {
		if c == cmd {
			return true
		}
	}
	return false
}

func TestStateWhenSystemIsDownIsStopped(t *testing.T) {
	m := &machine{systemUp: false}
	install(t, m)

	for _, stopSystem := range []bool{false, true} {
		if s, err := fizzy(stopSystem).State(); s != target.Stopped || err != nil {
			t.Fatalf("stop_system=%v: State() = %v, %v", stopSystem, s, err)
		}
	}
	if ran(m, "container list --all --format json") {
		t.Fatal("listed containers with the system down")
	}
}

func TestStateIsUnknownWhenProbesFail(t *testing.T) {
	for name, m := range map[string]*machine{
		"CLI missing":        {statusErr: errors.New("executable file not found")},
		"status fails oddly": {statusOut: "permission denied", statusCode: 2},
		"list isn't JSON":    {systemUp: true, listJSON: "Error: internalError"},
	} {
		install(t, m)
		if s, err := fizzy(false).State(); s != target.Unknown || err == nil {
			t.Fatalf("%s: State() = %v, %v", name, s, err)
		}
	}
}

// AS-18 and AS-29: running, partial and stopped stacks.
func TestStackStates(t *testing.T) {
	tests := []struct {
		states map[string]string
		want   target.State
	}{
		{map[string]string{"fizzy-db": "running", "fizzy-app": "running"}, target.Running},
		{map[string]string{"fizzy-db": "running", "fizzy-app": "stopped"}, target.Partial},
		{map[string]string{"fizzy-db": "stopped", "fizzy-app": "stopped"}, target.Stopped},
		{map[string]string{"fizzy-db": "stopping", "fizzy-app": "stopped"}, target.Partial},
		{map[string]string{"other": "running"}, target.Stopped},
	}
	for _, tt := range tests {
		install(t, &machine{systemUp: true, states: tt.states})
		if s, err := fizzy(false).State(); s != tt.want || err != nil {
			t.Fatalf("%v: State() = %v, %v; want %v", tt.states, s, err, tt.want)
		}
	}
}

// D-017: with stop_system, a running system makes a stopped stack partial for the eject rule,
// but the already-running guard still sees it as stopped.
func TestStopSystemMakesRunningSystemPartial(t *testing.T) {
	install(t, &machine{systemUp: true, states: map[string]string{"fizzy-db": "stopped", "other": "running"}})

	if s, _ := fizzy(true).State(); s != target.Partial {
		t.Fatalf("State() = %v, want partial", s)
	}
	if s, _ := fizzy(true).GuardState(); s != target.Stopped {
		t.Fatalf("GuardState() = %v, want stopped", s)
	}
	if s, _ := fizzy(false).State(); s != target.Stopped {
		t.Fatalf("without stop_system: State() = %v, want stopped", s)
	}
}

func TestStopRunsDownThenStopsTheSystem(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "running", "fizzy-app": "running"},
		downStops: []string{"fizzy-db", "fizzy-app"}}
	install(t, m)

	stopped, err := fizzy(true).Stop()
	if !stopped || err != nil || !ran(m, "/x/fizzy-down.sh") || !ran(m, "container system stop") {
		t.Fatalf("Stop() = %v, %v; ran %v", stopped, err, m.ran)
	}
	if s, _ := fizzy(true).State(); s != target.Stopped {
		t.Fatalf("after stop: State() = %v", s)
	}
}

func TestStopSystemRefusesWhileOtherContainersRun(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "running", "fizzy-app": "running", "buildkit": "running"},
		downStops: []string{"fizzy-db", "fizzy-app"}}
	install(t, m)

	stopped, err := fizzy(true).Stop()
	if !stopped || err == nil || !strings.Contains(err.Error(), "buildkit") || ran(m, "container system stop") {
		t.Fatalf("Stop() = %v, %v; ran %v", stopped, err, m.ran)
	}
}

func TestStopWithoutStopSystemLeavesTheSystem(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "running", "fizzy-app": "stopped"},
		downStops: []string{"fizzy-db"}}
	install(t, m)

	if stopped, err := fizzy(false).Stop(); !stopped || err != nil || ran(m, "container system stop") {
		t.Fatalf("Stop() = %v, %v; ran %v", stopped, err, m.ran)
	}
}

func TestStopWhenAlreadyStopped(t *testing.T) {
	m := &machine{systemUp: false}
	install(t, m)

	if stopped, err := fizzy(true).Stop(); stopped || err != nil || ran(m, "/x/fizzy-down.sh") || ran(m, "container system stop") {
		t.Fatalf("Stop() = %v, %v; ran %v", stopped, err, m.ran)
	}
}

func TestStopFailsWhenStateIsUnknown(t *testing.T) {
	m := &machine{statusOut: "permission denied", statusCode: 2}
	install(t, m)

	if stopped, err := fizzy(true).Stop(); stopped || err == nil || ran(m, "/x/fizzy-down.sh") {
		t.Fatalf("Stop() = %v, %v; ran %v", stopped, err, m.ran)
	}
}

func TestStartRunsUp(t *testing.T) {
	m := &machine{}
	install(t, m)
	if err := fizzy(false).Start(); err != nil || !ran(m, "/x/fizzy-up.sh") {
		t.Fatalf("Start() = %v; ran %v", err, m.ran)
	}
	m.scriptCode = 1
	if err := fizzy(false).Start(); err == nil {
		t.Fatal("Start() with a failing up script = nil")
	}
}

func TestChecks(t *testing.T) {
	install(t, &machine{})
	drive, home, scripts := t.TempDir(), t.TempDir(), t.TempDir()
	homeDir = func() (string, error) { return home, nil }

	up, down := filepath.Join(scripts, "up.sh"), filepath.Join(scripts, "down.sh")
	if err := os.WriteFile(up, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(down, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(drive, "container-data")
	if err := os.Mkdir(dataPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "Library", "Application Support"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dataPath, filepath.Join(home, "Library", "Application Support", "com.apple.container")); err != nil {
		t.Fatal(err)
	}

	tg := New(config.Target{Name: "fizzy", Container: &config.Container{Up: up, Down: down, DataPath: dataPath,
		Containers: []string{"fizzy-db"}}}, drive)
	items := tg.Checks()
	for _, item := range items {
		wantOK := item.Name != "down script executable"
		if item.OK != wantOK {
			t.Fatalf("%s: OK = %v (%s)", item.Name, item.OK, item.Message)
		}
	}
	if checks.AllOK(items) {
		t.Fatal("a non-executable down script passed")
	}
}

// Review on PR #9: only a confirmed "unregistered" status means stopped. A registered daemon that
// fails its health check (a status other than running, or the plain-text "is not running") is
// unknown, which blocks eject.
func TestOnlyUnregisteredSystemIsStopped(t *testing.T) {
	for name, m := range map[string]*machine{
		"health check failed (text)": {statusOut: "apiserver is not running", statusCode: 1},
		"registered, not running":    {statusOut: `{"status":"registered"}`, statusCode: 1},
		"unhealthy":                  {statusOut: `{"status":"unhealthy"}`, statusCode: 1},
		"no status field":            {statusOut: `{}`, statusCode: 1},
		"running but exit 1":         {statusOut: `{"status":"running"}`, statusCode: 1},
	} {
		install(t, m)
		if s, err := fizzy(true).State(); s != target.Unknown || err == nil {
			t.Fatalf("%s: State() = %v, %v; want unknown", name, s, err)
		}
	}
}

// Review on PR #9: list entries without an id or a state can't be trusted, so the list is
// unreadable and the state unknown, instead of a running entry being lost.
func TestListEntriesMustHaveIDAndState(t *testing.T) {
	for name, list := range map[string]string{
		"missing id":     `[{"status":{"state":"running"}},{"id":"fizzy-app","status":{"state":"stopped"}}]`,
		"empty id":       `[{"id":"","status":{"state":"running"}}]`,
		"missing state":  `[{"id":"fizzy-db","status":{}}]`,
		"missing status": `[{"id":"fizzy-db"}]`,
		"not a list":     `{"id":"fizzy-db"}`,
	} {
		install(t, &machine{systemUp: true, listJSON: list})
		if s, err := fizzy(false).State(); s != target.Unknown || err == nil {
			t.Fatalf("%s: State() = %v, %v; want unknown", name, s, err)
		}
	}
}
