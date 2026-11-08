package app

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/target"
)

// fake is a target whose probe answers come from a list; the last answer repeats.
type fake struct {
	name      string
	kind      string
	states    []target.State
	stateErr  error
	probes    int
	checksOK  bool
	started   bool
	stopped   bool
	stopErr   error
	noChecks  bool
	noProbing bool
	t         *testing.T
}

func (f *fake) Name() string     { return f.name }
func (f *fake) Kind() string     { return f.kind }
func (f *fake) MainPath() string { return "/Volumes/X/" + f.name }
func (f *fake) Checks() []checks.Item {
	if f.noChecks {
		f.t.Fatalf("%s: Checks() called", f.name)
	}
	return []checks.Item{{Name: f.name + " check", OK: f.checksOK}}
}
func (f *fake) State() (target.State, error) {
	if f.noProbing {
		f.t.Fatalf("%s: State() called", f.name)
	}
	i := f.probes
	if i >= len(f.states) {
		i = len(f.states) - 1
	}
	f.probes++
	if f.states[i] == target.Unknown {
		return target.Unknown, f.stateErr
	}
	return f.states[i], nil
}
func (f *fake) Start() error { f.started = true; return nil }
func (f *fake) Stop() (bool, error) {
	if f.stopErr != nil {
		return false, f.stopErr
	}
	f.stopped = true
	return true, nil
}

type harness struct {
	out     *bytes.Buffer
	ejected bool
	slept   time.Duration
}

// setup installs fakes for the given targets. Every target has fail_if_already_running on.
func setup(t *testing.T, mounted bool, legacy bool, fakes ...*fake) *harness {
	t.Helper()
	h := &harness{out: &bytes.Buffer{}}

	// The drive is a real folder, so the drive check (C1) passes when "mounted".
	cfg := config.Config{Path: "/home/me/.config/dockguard/dockguard.yaml", Drive: t.TempDir(), Legacy: legacy}
	var entries []entry
	for _, f := range fakes {
		f.t = t
		if f.kind == "" {
			f.kind = "fake"
		}
		tc := config.Target{Name: f.name, Kind: f.kind, FailIfAlreadyRunning: true}
		cfg.Targets = append(cfg.Targets, tc)
		entries = append(entries, entry{t: f, cfg: tc})
	}

	orig := struct {
		load   func(string) (config.Config, error)
		build  func(config.Config) ([]entry, error)
		mount  func(string) bool
		eject  func(string) error
		sleep  func(time.Duration)
		now    func() time.Time
		output io.Writer
	}{loadConfig, buildTargets, driveMounted, ejectVolume, sleep, now, output.Out}
	t.Cleanup(func() {
		loadConfig, buildTargets, driveMounted, ejectVolume, sleep, now = orig.load, orig.build, orig.mount, orig.eject, orig.sleep, orig.now
		output.Out = orig.output
	})

	clock := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	driveMounted = func(string) bool { return mounted }
	ejectVolume = func(string) error { h.ejected = true; return nil }
	sleep = func(d time.Duration) { h.slept += d; clock = clock.Add(d) }
	now = func() time.Time { return clock }
	output.Out = h.out
	return h
}

func states(s ...target.State) []target.State { return s }

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    options
		wantErr string
	}{
		{[]string{"start"}, options{Command: "start"}, ""},
		{[]string{"start", "postgres", "--config", "c.yaml"}, options{Command: "start", Target: "postgres", ConfigPath: "c.yaml"}, ""},
		{[]string{"stop", "--eject", "docker"}, options{Command: "stop", Target: "docker", Eject: true}, ""},
		{[]string{"list", "docker"}, options{}, "unexpected argument"},
		{[]string{"start", "a", "b"}, options{}, "unexpected argument"},
		{[]string{"check", "--eject"}, options{}, "only supported with the stop command"},
		{[]string{"launch"}, options{}, "unknown command"},
		{[]string{"status", "--verbose"}, options{}, "unknown argument"},
	}
	for _, tt := range tests {
		got, err := parseArgs(tt.args)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseArgs(%v) error = %v, want %q", tt.args, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Fatalf("parseArgs(%v) = %#v, %v; want %#v", tt.args, got, err, tt.want)
		}
	}
}

// AS-05: list shows every target in config order, with its state, and runs no checks.
func TestListShowsTargetsWithoutChecks(t *testing.T) {
	h := setup(t, true, false,
		&fake{name: "docker", states: states(target.Running), noChecks: true},
		&fake{name: "fizzy", states: states(target.Partial), noChecks: true},
	)

	code, err := Run([]string{"list"})
	if code != 0 || err != nil {
		t.Fatalf("Run = %d, %v", code, err)
	}
	out := h.out.String()
	for _, want := range []string{"Config: /home/me/.config/dockguard/dockguard.yaml", "(mounted)", "running", "partial"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "docker") > strings.Index(out, "fizzy") {
		t.Fatalf("targets not in config order:\n%s", out)
	}
}

// AS-06: with the drive unmounted, list shows every target as "drive not mounted" and exits 0.
func TestListWithDriveUnmounted(t *testing.T) {
	h := setup(t, false, false, &fake{name: "docker", noChecks: true, noProbing: true})

	code, err := Run([]string{"list"})
	if code != 0 || err != nil {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !strings.Contains(h.out.String(), "drive not mounted") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// AS-07: a running target is a state in status, not a failure.
func TestStatusDoesNotCountRunningAsNotReady(t *testing.T) {
	h := setup(t, true, false, &fake{name: "docker", states: states(target.Running), checksOK: true})

	code, _ := Run([]string{"status"})
	if code != 0 || !strings.Contains(h.out.String(), "Status: ready") {
		t.Fatalf("code = %d, output:\n%s", code, h.out)
	}
}

func TestStatusFailsWhenAChecksFails(t *testing.T) {
	h := setup(t, true, false,
		&fake{name: "docker", states: states(target.Stopped), checksOK: true},
		&fake{name: "other", states: states(target.Stopped), checksOK: false},
	)

	code, _ := Run([]string{"status"})
	if code != 1 || !strings.Contains(h.out.String(), "Status: not ready") {
		t.Fatalf("code = %d, output:\n%s", code, h.out)
	}
}

// AS-02: a v0.1 config works and status suggests the new format.
func TestStatusSuggestsNewFormatForLegacyConfig(t *testing.T) {
	h := setup(t, true, true, &fake{name: "docker", states: states(target.Stopped), checksOK: true})

	if code, _ := Run([]string{"status"}); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(h.out.String(), "v0.1 format") {
		t.Fatalf("no legacy notice:\n%s", h.out)
	}
}

func TestChecksAreSkippedWhenDriveIsUnmounted(t *testing.T) {
	h := setup(t, false, false, &fake{name: "docker", checksOK: true, noProbing: true})

	code, _ := Run([]string{"check"})
	if code != 1 || !strings.Contains(h.out.String(), "[skip] docker check (skipped: drive not mounted)") {
		t.Fatalf("code = %d, output:\n%s", code, h.out)
	}
}

// AS-08: start refuses when the target is already running and the guard is on.
func TestStartRefusesWhenAlreadyRunning(t *testing.T) {
	f := &fake{name: "docker", kind: config.KindDockerDesktop, states: states(target.Running), checksOK: true}
	h := setup(t, true, false, f)

	code, err := Run([]string{"start"})
	if code != 1 || err == nil || f.started {
		t.Fatalf("Run = %d, %v; started = %v", code, err, f.started)
	}
	if !strings.Contains(h.out.String(), "docker desktop not already running (Docker Desktop is already running)") {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestStartGuardFailsOnUnknownState(t *testing.T) {
	f := &fake{name: "pg", states: states(target.Unknown), stateErr: errors.New("pg_ctl not found"), checksOK: true}
	h := setup(t, true, false, f)

	if code, _ := Run([]string{"start", "pg"}); code != 1 || f.started {
		t.Fatalf("code = %d, started = %v", code, f.started)
	}
	if !strings.Contains(h.out.String(), "pg_ctl not found") {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestStartRunsWhenChecksPass(t *testing.T) {
	f := &fake{name: "docker", states: states(target.Stopped), checksOK: true}
	setup(t, true, false, f)

	if code, err := Run([]string{"start"}); code != 0 || err != nil || !f.started {
		t.Fatalf("Run = %d, %v; started = %v", code, err, f.started)
	}
}

// AS-09: an unknown target name fails and lists the valid names.
func TestUnknownTarget(t *testing.T) {
	setup(t, true, false, &fake{name: "docker"}, &fake{name: "fizzy"})

	code, err := Run([]string{"start", "postgres"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "docker, fizzy") {
		t.Fatalf("Run = %d, %v", code, err)
	}
}

func TestStopEjectsWhenEverythingIsConfirmedStopped(t *testing.T) {
	// Docker reports running once after the stop, then stopped.
	docker := &fake{name: "docker", states: states(target.Running, target.Stopped)}
	other := &fake{name: "other", states: states(target.Stopped)}
	h := setup(t, true, false, docker, other)

	code, err := Run([]string{"stop", "--eject"})
	if code != 0 || err != nil || !h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v\n%s", code, err, h.ejected, h.out)
	}
}

// AS-10: another target on the drive is running, so the drive isn't ejected.
func TestStopDoesNotEjectWhileAnotherTargetRuns(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	pg := &fake{name: "pg", states: states(target.Running)}
	h := setup(t, true, false, docker, pg)

	code, _ := Run([]string{"stop", "--eject"})
	if code != 1 || h.ejected || !docker.stopped {
		t.Fatalf("code = %d, ejected = %v, docker stopped = %v", code, h.ejected, docker.stopped)
	}
	if !strings.Contains(h.out.String(), "- pg: running") {
		t.Fatalf("output:\n%s", h.out)
	}
	if h.slept != 0 {
		t.Fatalf("waited %v for a target that wasn't just stopped", h.slept)
	}
}

func TestStopDoesNotEjectWhileAStackIsPartial(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	fizzy := &fake{name: "fizzy", states: states(target.Partial)}
	h := setup(t, true, false, docker, fizzy)

	if code, _ := Run([]string{"stop", "--eject"}); code != 1 || h.ejected {
		t.Fatalf("code = %d, ejected = %v", code, h.ejected)
	}
	if !strings.Contains(h.out.String(), "- fizzy: partial") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// AS-27: a failed probe means unknown, which blocks the eject and shows the probe error.
func TestStopDoesNotEjectWhenAProbeFails(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	pg := &fake{name: "pg", states: states(target.Unknown), stateErr: errors.New("pg_ctl: could not read status")}
	h := setup(t, true, false, docker, pg)

	if code, _ := Run([]string{"stop", "--eject"}); code != 1 || h.ejected {
		t.Fatalf("code = %d, ejected = %v", code, h.ejected)
	}
	if !strings.Contains(h.out.String(), "- pg: unknown (pg_ctl: could not read status)") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// AS-28: the stop was requested but the target is still running 30 seconds later.
func TestStopDoesNotEjectWhenStopDoesNotTakeEffect(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Running)}
	h := setup(t, true, false, docker)

	if code, _ := Run([]string{"stop", "--eject"}); code != 1 || h.ejected {
		t.Fatalf("code = %d, ejected = %v", code, h.ejected)
	}
	if h.slept < stopConfirmTimeout {
		t.Fatalf("waited %v, want at least %v", h.slept, stopConfirmTimeout)
	}
	if !strings.Contains(h.out.String(), "- docker: running") {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestStopErrorSkipsEject(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Running), stopErr: errors.New("docker CLI not found")}
	h := setup(t, true, false, docker)

	if code, err := Run([]string{"stop", "--eject"}); code != 1 || err == nil || h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v", code, err, h.ejected)
	}
}

func TestStopWithoutEjectDoesNotProbeOthers(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	other := &fake{name: "other", noProbing: true}
	h := setup(t, true, false, docker, other)

	if code, err := Run([]string{"stop"}); code != 0 || err != nil || h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v", code, err, h.ejected)
	}
}

func TestEjectRefusedWhenDriveNotMounted(t *testing.T) {
	h := setup(t, false, false, &fake{name: "docker", states: states(target.Stopped)})

	if code, err := Run([]string{"stop", "--eject"}); code != 1 || err == nil || h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v", code, err, h.ejected)
	}
}

// Review on PR #4: a target whose state can't be read isn't ready, even if its checks pass.
func TestStatusFailsWhenAStateProbeFails(t *testing.T) {
	h := setup(t, true, false, &fake{name: "docker", states: states(target.Unknown), stateErr: errors.New("docker desktop status failed"), checksOK: true})

	code, _ := Run([]string{"status"})
	if code != 1 || !strings.Contains(h.out.String(), "Status: not ready") {
		t.Fatalf("code = %d, output:\n%s", code, h.out)
	}
	if !strings.Contains(h.out.String(), "unknown (docker desktop status failed)") {
		t.Fatalf("output:\n%s", h.out)
	}
}
