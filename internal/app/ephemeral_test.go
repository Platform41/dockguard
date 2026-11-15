package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/target"
	containerkind "github.com/platform41/dockguard/internal/target/container"
	"github.com/platform41/dockguard/internal/ui"
)

type ephemeralFake struct {
	*fake
	stackRunning bool
	systemUp     bool
	discardCalls int
}

func (f *ephemeralFake) State() (target.State, error) {
	switch {
	case f.stackRunning:
		return target.Running, nil
	case f.systemUp:
		return target.Partial, nil
	default:
		return target.Stopped, nil
	}
}

func (f *ephemeralFake) StopStack() (bool, error) {
	if !f.stackRunning {
		return false, nil
	}
	f.discardCalls++
	output.PrintEphemeralDiscard(f.name)
	f.stackRunning = false
	return true, nil
}

func (f *ephemeralFake) StopSystem() (bool, error) {
	if !f.systemUp {
		return false, nil
	}
	f.systemUp = false
	return true, nil
}

func (f *ephemeralFake) Stop() (bool, error) {
	stack, err := f.StopStack()
	if err != nil {
		return false, err
	}
	system, err := f.StopSystem()
	return stack || system, err
}

func newEphemeralFake() *ephemeralFake {
	return &ephemeralFake{fake: &fake{name: "aws", kind: config.KindContainer, checksOK: true}}
}

func setupEphemeral(t *testing.T, f *ephemeralFake) *harness {
	t.Helper()
	h := setup(t, true, false, f.fake)
	buildTargets = func(cfg config.Config) ([]entry, error) {
		tc := cfg.Targets[0]
		tc.Container = &config.Container{Ephemeral: true}
		return []entry{{t: f, cfg: tc}}, nil
	}
	return h
}

// AS-53: list marks ephemeral state, while status headings stay unchanged.
func TestEphemeralListMarkerAndUnchangedStatusAS53(t *testing.T) {
	f := newEphemeralFake()
	f.stackRunning = true
	h := setupEphemeral(t, f)

	if code, err := Run([]string{"list"}); code != 0 || err != nil ||
		!strings.Contains(h.out.String(), "running (ephemeral)") {
		t.Fatalf("list = %d, %v, output:\n%s", code, err, h.out)
	}
	h.out.Reset()
	if code, err := Run([]string{"status"}); code != 0 || err != nil {
		t.Fatalf("status = %d, %v", code, err)
	}
	if !strings.Contains(h.out.String(), "aws (container): running\n") || strings.Contains(h.out.String(), "(ephemeral)") {
		t.Fatalf("status heading changed:\n%s", h.out)
	}

	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	snapshot := (&controller{cfg: cfg, entries: entries}).Snapshot()
	if len(snapshot.Rows) != 1 || !snapshot.Rows[0].Ephemeral {
		t.Fatalf("dashboard row = %#v", snapshot.Rows)
	}
}

// AS-54: single stop, stop --all, and dashboard stop print the notice once before stop output.
func TestEphemeralStopNoticeAS54(t *testing.T) {
	for _, args := range [][]string{{"stop", "aws"}, {"stop", "--all"}} {
		f := newEphemeralFake()
		f.stackRunning = true
		h := setupEphemeral(t, f)
		code, err := Run(args)
		want := "aws is ephemeral: its data is discarded on stop.\naws stop requested.\n"
		if code != 0 || err != nil || f.discardCalls != 1 || h.out.String() != want {
			t.Fatalf("%v = %d, %v; callbacks = %d, output %q, want %q", args, code, err, f.discardCalls, h.out.String(), want)
		}
	}

	f := newEphemeralFake()
	f.stackRunning = true
	h := setupEphemeral(t, f)
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	ok, out := (&controller{cfg: cfg, entries: entries}).Run(ui.Stop, "aws")
	want := "aws is ephemeral: its data is discarded on stop.\naws stop requested.\n"
	if !ok || out != want || h.out.Len() != 0 {
		t.Fatalf("dashboard stop = %v, output %q, leaked %q", ok, out, h.out)
	}
}

// AS-54: the real container callback resolves output.Out at call time for dashboard capture.
func TestContainerDiscardCallbackUsesCurrentOutputAS54(t *testing.T) {
	original := output.Out
	t.Cleanup(func() { output.Out = original })
	cfg := config.Config{Drive: "/Volumes/X", Targets: []config.Target{{
		Name: "aws", Kind: config.KindContainer, Container: &config.Container{Ephemeral: true},
	}}}
	entries, err := defaultBuildTargets(cfg)
	if err != nil || len(entries) != 1 {
		t.Fatalf("defaultBuildTargets = %v, %v", entries, err)
	}
	tg, ok := entries[0].t.(*containerkind.Target)
	if !ok || tg.OnDiscard == nil {
		t.Fatalf("container callback is not wired: %#v", entries[0].t)
	}

	var first, second bytes.Buffer
	output.Out = &first
	tg.OnDiscard("aws")
	output.Out = &second
	tg.OnDiscard("aws")
	want := "aws is ephemeral: its data is discarded on stop.\n"
	if first.String() != want || second.String() != want {
		t.Fatalf("callback outputs = %q and %q, want %q each", first.String(), second.String(), want)
	}
}

// AS-55: ephemeral does not change stop confirmation or the eject rule.
func TestEphemeralEjectReadinessUnchangedAS55(t *testing.T) {
	f := newEphemeralFake()
	f.stackRunning = true
	h := setupEphemeral(t, f)

	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 0 || err != nil || !h.ejected || f.discardCalls != 1 {
		t.Fatalf("stop --all --eject = %d, %v; ejected = %v, callbacks = %d\n%s", code, err, h.ejected, f.discardCalls, h.out)
	}
}

// AS-56: fully stopped output is byte-for-byte unchanged and has no callback.
func TestEphemeralFullyStoppedOutputAS56(t *testing.T) {
	f := newEphemeralFake()
	h := setupEphemeral(t, f)

	code, err := Run([]string{"stop", "aws"})
	if code != 0 || err != nil || f.discardCalls != 0 || h.out.String() != "aws is already stopped.\n" {
		t.Fatalf("stop = %d, %v; callbacks = %d, output %q", code, err, f.discardCalls, h.out.String())
	}
}

// AS-58: system-only single stop keeps its original output and emits no callback.
func TestEphemeralSingleSystemOnlyOutputAS58(t *testing.T) {
	f := newEphemeralFake()
	f.systemUp = true
	h := setupEphemeral(t, f)

	code, err := Run([]string{"stop", "aws"})
	if code != 0 || err != nil || f.discardCalls != 0 || h.out.String() != "aws stop requested.\n" {
		t.Fatalf("stop = %d, %v; callbacks = %d, output %q", code, err, f.discardCalls, h.out.String())
	}
}

// AS-59: system-only stop --all keeps both original lines and emits no callback.
func TestEphemeralStopAllSystemOnlyOutputAS59(t *testing.T) {
	f := newEphemeralFake()
	f.systemUp = true
	h := setupEphemeral(t, f)

	code, err := Run([]string{"stop", "--all"})
	want := "aws is already stopped.\naws: container system stop requested.\n"
	if code != 0 || err != nil || f.discardCalls != 0 || h.out.String() != want {
		t.Fatalf("stop --all = %d, %v; callbacks = %d, output %q, want %q", code, err, f.discardCalls, h.out.String(), want)
	}
}
