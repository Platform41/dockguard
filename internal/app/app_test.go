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
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
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
		load    func(string) (config.Config, error)
		build   func(config.Config) ([]entry, error)
		mount   func(string) bool
		eject   func(string) error
		holders func(string) ([]platform.Holder, error)
		sleep   func(time.Duration)
		now     func() time.Time
		output  io.Writer
	}{
		load: loadConfig, build: buildTargets, mount: driveMounted, eject: ejectVolume,
		holders: holdersOf, sleep: sleep, now: now, output: output.Out,
	}
	t.Cleanup(func() {
		loadConfig, buildTargets, driveMounted, ejectVolume, holdersOf, sleep, now = orig.load, orig.build, orig.mount, orig.eject, orig.holders, orig.sleep, orig.now
		output.Out = orig.output
	})

	clock := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	driveMounted = func(string) bool { return mounted }
	ejectVolume = func(string) error { h.ejected = true; return nil }
	holdersOf = func(string) ([]platform.Holder, error) { return nil, nil }
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

// AS-41: known process holders prevent diskutil from being called.
func TestStopEjectRefusesKnownProcessHolder(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	h := setup(t, true, false, docker)
	holdersOf = func(string) ([]platform.Holder, error) {
		return []platform.Holder{{Name: "zsh", PID: 23721}}, nil
	}

	code, err := Run([]string{"stop", "--eject"})
	if code != 1 || err == nil || h.ejected || !strings.Contains(h.out.String(), "zsh (PID 23721)") {
		t.Fatalf("Run = %d, %v; ejected = %v\n%s", code, err, h.ejected, h.out)
	}
}

// AS-39: an incomplete holder scan is unknown and blocks eject.
func TestStopEjectRefusesHolderInspectionFailure(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Stopped)}
	h := setup(t, true, false, docker)
	holdersOf = func(string) ([]platform.Holder, error) { return nil, errors.New("lsof timed out") }

	code, err := Run([]string{"stop", "--eject"})
	if code != 1 || err == nil || h.ejected || !strings.Contains(h.out.String(), "lsof timed out") {
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

// AS-12: with the drive unmounted, start refuses and the app isn't opened.
func TestStartRefusesWhenDriveIsUnmounted(t *testing.T) {
	f := &fake{name: "xcode", checksOK: true, noProbing: true}
	h := setup(t, false, false, f)

	code, err := Run([]string{"start", "xcode"})
	if code != 1 || err == nil || f.started {
		t.Fatalf("Run = %d, %v; started = %v\n%s", code, err, f.started, h.out)
	}
}

// guarded is a fake whose already-running guard looks at something narrower than State.
type guarded struct {
	*fake
	guardState target.State
}

func (g *guarded) GuardState() (target.State, error) { return g.guardState, nil }

// D-017: a container system kept up by something else makes the stack partial for eject, but
// doesn't block start.
func TestStartUsesGuardState(t *testing.T) {
	f := &fake{name: "fizzy", states: states(target.Partial), checksOK: true}
	g := &guarded{fake: f, guardState: target.Stopped}
	setup(t, true, false, f)
	buildTargets = func(cfg config.Config) ([]entry, error) {
		return []entry{{t: g, cfg: cfg.Targets[0]}}, nil
	}

	if code, err := Run([]string{"start", "fizzy"}); code != 0 || err != nil || !f.started {
		t.Fatalf("Run = %d, %v; started = %v", code, err, f.started)
	}
}

// stopOrder records the order fakes are stopped in.
type ordered struct {
	*fake
	order *[]string
}

func (o *ordered) Stop() (bool, error) {
	*o.order = append(*o.order, o.name)
	return o.fake.Stop()
}

// setupOrdered is setup with fakes that record their stop order.
func setupOrdered(t *testing.T, fakes ...*fake) (*harness, *[]string) {
	t.Helper()
	h := setup(t, true, false, fakes...)
	order := &[]string{}
	buildTargets = func(cfg config.Config) ([]entry, error) {
		var entries []entry
		for i, f := range fakes {
			entries = append(entries, entry{t: &ordered{fake: f, order: order}, cfg: cfg.Targets[i]})
		}
		return entries, nil
	}
	return h, order
}

func TestParseArgsAll(t *testing.T) {
	if got, err := parseArgs([]string{"stop", "--all", "--eject"}); err != nil || !got.All || !got.Eject {
		t.Fatalf("parseArgs = %#v, %v", got, err)
	}
	for _, args := range [][]string{{"start", "--all"}, {"stop", "docker", "--all"}} {
		if _, err := parseArgs(args); err == nil {
			t.Fatalf("parseArgs(%v) error = nil", args)
		}
	}
}

// AS-19: everything running; stop --all --eject stops in reverse config order, then ejects.
func TestStopAllStopsInReverseOrderThenEjects(t *testing.T) {
	h, order := setupOrdered(t,
		&fake{name: "docker", states: states(target.Running, target.Stopped)},
		&fake{name: "pg", states: states(target.Running, target.Stopped)},
		&fake{name: "fizzy", states: states(target.Partial, target.Stopped)},
	)

	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 0 || err != nil || !h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v\n%s", code, err, h.ejected, h.out)
	}
	if got := strings.Join(*order, ","); got != "fizzy,pg,docker" {
		t.Fatalf("stop order = %s", got)
	}
}

func TestStopAllSkipsStoppedTargets(t *testing.T) {
	h, order := setupOrdered(t,
		&fake{name: "docker", states: states(target.Stopped)},
		&fake{name: "pg", states: states(target.Running, target.Stopped)},
	)

	if code, err := Run([]string{"stop", "--all"}); code != 0 || err != nil || h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v", code, err, h.ejected)
	}
	if got := strings.Join(*order, ","); got != "pg" {
		t.Fatalf("stopped %s, want only pg", got)
	}
	if !strings.Contains(h.out.String(), "Docker") && !strings.Contains(h.out.String(), "docker is already stopped") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// AS-20: one target fails to stop; the others are still stopped, and the drive isn't ejected.
func TestStopAllCarriesOnButDoesNotEjectAfterAFailure(t *testing.T) {
	h, order := setupOrdered(t,
		&fake{name: "docker", states: states(target.Running, target.Stopped)},
		&fake{name: "pg", states: states(target.Running), stopErr: errors.New("pg_ctl: server does not shut down")},
		&fake{name: "fizzy", states: states(target.Running, target.Stopped)},
	)

	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 1 || err == nil || h.ejected {
		t.Fatalf("Run = %d, %v; ejected = %v", code, err, h.ejected)
	}
	if got := strings.Join(*order, ","); got != "fizzy,pg,docker" {
		t.Fatalf("stop order = %s; the failure should not stop the rest", got)
	}
	if !strings.Contains(h.out.String(), "failed to stop: pg") || !strings.Contains(h.out.String(), "Not ejecting") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// AS-30: a target still unknown after stop --all blocks the eject, with the probe error.
func TestStopAllDoesNotEjectWhileATargetStaysUnknown(t *testing.T) {
	h, _ := setupOrdered(t,
		&fake{name: "docker", states: states(target.Running, target.Stopped)},
		&fake{name: "fizzy", states: states(target.Unknown), stateErr: errors.New("container system status: exit 1, status \"registered\"")},
	)

	if code, _ := Run([]string{"stop", "--all", "--eject"}); code != 1 || h.ejected {
		t.Fatalf("code = %d, ejected = %v", code, h.ejected)
	}
	if !strings.Contains(h.out.String(), "fizzy: unknown") {
		t.Fatalf("output:\n%s", h.out)
	}
}

// sharedSystem is a fake container system that two stacks run on.
type sharedSystem struct {
	up        bool
	running   map[string]bool // stack name -> its containers running
	unmanaged bool            // a container outside both stacks is running
}

// stack is a fake container target with stop_system, on a shared system.
type stack struct {
	*fake
	sys *sharedSystem
}

func (s *stack) State() (target.State, error) {
	switch {
	case s.sys.running[s.name]:
		return target.Running, nil
	case s.sys.up:
		return target.Partial, nil // stop_system: a running system holds the drive
	default:
		return target.Stopped, nil
	}
}

func (s *stack) StopStack() (bool, error) {
	was := s.sys.running[s.name]
	s.sys.running[s.name] = false
	return was, nil
}

func (s *stack) StopSystem() (bool, error) {
	if !s.sys.up {
		return false, nil
	}
	for name, up := range s.sys.running {
		if up && name != s.name {
			return false, errors.New("not stopping the container system: other containers are running (" + name + ")")
		}
	}
	if s.sys.unmanaged {
		return false, errors.New("not stopping the container system: other containers are running (buildkit)")
	}
	s.sys.up = false
	return true, nil
}

// Stop is the single-target path: stack, then system.
func (s *stack) Stop() (bool, error) {
	stopped, _ := s.StopStack()
	sys, err := s.StopSystem()
	return stopped || sys, err
}

func setupStacks(t *testing.T, sys *sharedSystem) *harness {
	t.Helper()
	a := &fake{name: "stack-a"}
	b := &fake{name: "stack-b"}
	h := setup(t, true, false, a, b)
	buildTargets = func(cfg config.Config) ([]entry, error) {
		return []entry{
			{t: &stack{fake: a, sys: sys}, cfg: cfg.Targets[0]},
			{t: &stack{fake: b, sys: sys}, cfg: cfg.Targets[1]},
		}, nil
	}
	return h
}

// Review on PR #10: two stacks share the container system. stop --all stops both stacks first,
// then the system, so neither stack's shutdown is refused because of the other.
func TestStopAllStopsSharedSystemAfterEveryStack(t *testing.T) {
	sys := &sharedSystem{up: true, running: map[string]bool{"stack-a": true, "stack-b": true}}
	h := setupStacks(t, sys)

	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 0 || err != nil || sys.up || !h.ejected {
		t.Fatalf("Run = %d, %v; system up = %v, ejected = %v\n%s", code, err, sys.up, h.ejected, h.out)
	}
}

// Unmanaged containers still protect the shared system.
func TestStopAllKeepsSystemWhileUnmanagedContainersRun(t *testing.T) {
	sys := &sharedSystem{up: true, running: map[string]bool{"stack-a": true, "stack-b": true}, unmanaged: true}
	h := setupStacks(t, sys)

	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 1 || err == nil || !sys.up || h.ejected || sys.running["stack-a"] || sys.running["stack-b"] {
		t.Fatalf("Run = %d, %v; system up = %v, ejected = %v, stacks %v\n%s", code, err, sys.up, h.ejected, sys.running, h.out)
	}
	if !strings.Contains(h.out.String(), "buildkit") {
		t.Fatalf("output should name the unmanaged container:\n%s", h.out)
	}
}

// AS-35: without a terminal, ui exits 1 and points to list and status.
func TestUIRefusesWithoutATerminal(t *testing.T) {
	setup(t, true, false, &fake{name: "docker", states: states(target.Stopped)})
	orig := isTerminal
	t.Cleanup(func() { isTerminal = orig })
	isTerminal = func() bool { return false }

	code, err := Run([]string{"ui"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "dockguard list") {
		t.Fatalf("Run = %d, %v", code, err)
	}
}

func TestUIOpensTheDashboardWithAController(t *testing.T) {
	setup(t, true, false, &fake{name: "docker", states: states(target.Running)})
	origTerm, origUI := isTerminal, runUI
	t.Cleanup(func() { isTerminal, runUI = origTerm, origUI })
	isTerminal = func() bool { return true }

	var got ui.Snapshot
	runUI = func(ctl ui.Controller) error { got = ctl.Snapshot(); return nil }
	if code, err := Run([]string{"ui"}); code != 0 || err != nil {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if !got.Mounted || len(got.Rows) != 1 || got.Rows[0].State != target.Running {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestDashboardSnapshotIncludesHolders(t *testing.T) {
	setup(t, true, false, &fake{name: "docker", states: states(target.Stopped)})
	holdersOf = func(string) ([]platform.Holder, error) {
		return []platform.Holder{{Name: "zsh", PID: 23721}}, errors.New("inspection incomplete")
	}
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)

	got := (&controller{cfg: cfg, entries: entries}).Snapshot()
	if len(got.Holders) != 1 || got.Holders[0] != (ui.Holder{Name: "zsh", PID: 23721}) ||
		!strings.Contains(got.HolderErr, "inspection incomplete") {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestDashboardSnapshotSkipsHoldersWhenDriveIsUnmounted(t *testing.T) {
	setup(t, false, false, &fake{name: "docker", noProbing: true})
	called := false
	holdersOf = func(string) ([]platform.Holder, error) { called = true; return nil, nil }
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)

	got := (&controller{cfg: cfg, entries: entries}).Snapshot()
	if called || got.Mounted {
		t.Fatalf("holder scan called = %v, snapshot = %#v", called, got)
	}
}

func TestReleaseDriveCWDOnlyWhenInsideDrive(t *testing.T) {
	origGetwd, origChdir := getwd, chdir
	t.Cleanup(func() { getwd, chdir = origGetwd, origChdir })
	var changedTo string
	chdir = func(path string) error { changedTo = path; return nil }

	getwd = func() (string, error) { return "/Volumes/X/projects/app", nil }
	if err := releaseDriveCWD("/Volumes/X"); err != nil || changedTo != "/" {
		t.Fatalf("inside: changed to %q, err %v", changedTo, err)
	}

	changedTo = ""
	getwd = func() (string, error) { return "/Volumes/X-other", nil }
	if err := releaseDriveCWD("/Volumes/X"); err != nil || changedTo != "" {
		t.Fatalf("sibling: changed to %q, err %v", changedTo, err)
	}
}

// The dashboard's actions are the CLI's: output is captured, and the eject rule still applies.
func TestControllerRunsTheCLIPaths(t *testing.T) {
	docker := &fake{name: "docker", states: states(target.Running), checksOK: true}
	h := setup(t, true, false, docker)
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	c := &controller{cfg: cfg, entries: entries}

	ok, out := c.Run(ui.Check, "docker")
	if ok || !strings.Contains(out, "docker desktop not already running") && !strings.Contains(out, "docker not already running") {
		t.Fatalf("check = %v, %q", ok, out)
	}
	ok, out = c.Run(ui.StopAllEject, "")
	if ok || h.ejected || !strings.Contains(out, "Not ejecting") {
		t.Fatalf("stop all + eject = %v, %q; ejected %v", ok, out, h.ejected)
	}
	if h.out.Len() != 0 {
		t.Fatalf("controller output leaked to stdout: %q", h.out.String())
	}
}
