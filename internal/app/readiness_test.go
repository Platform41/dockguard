package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
	containerkind "github.com/platform41/dockguard/internal/target/container"
	"github.com/platform41/dockguard/internal/ui"
)

type readinessFake struct {
	*fake
	elapsed        time.Duration
	startErr       error
	readinessCalls int
}

func (f *readinessFake) Checks() []checks.Item {
	return []checks.Item{{Name: "ready script executable", OK: f.checksOK}}
}

func (f *readinessFake) StartWithReadiness() (time.Duration, error) {
	f.readinessCalls++
	f.started = true
	return f.elapsed, f.startErr
}

func setupReadiness(t *testing.T, f *readinessFake, ready bool) *harness {
	t.Helper()
	h := setup(t, true, false, f.fake)
	buildTargets = func(cfg config.Config) ([]entry, error) {
		tc := cfg.Targets[0]
		if ready {
			tc.Container = &config.Container{Ready: "/x/ready.sh", ReadyTimeoutSeconds: 30}
		}
		return []entry{{t: f, cfg: tc}}, nil
	}
	return h
}

func newReadinessFake() *readinessFake {
	return &readinessFake{fake: &fake{
		name: "fizzy", kind: config.KindContainer, states: states(target.Stopped), checksOK: true,
	}}
}

// AS-46: the shared CLI start path reports a successful readiness result.
func TestStartReportsReadyAS46(t *testing.T) {
	f := newReadinessFake()
	f.elapsed = 2100 * time.Millisecond
	h := setupReadiness(t, f, true)

	code, err := Run([]string{"start", "fizzy"})
	if code != 0 || err != nil || f.readinessCalls != 1 ||
		!strings.Contains(h.out.String(), "fizzy started and ready (2.1 s).") {
		t.Fatalf("Run = %d, %v; calls = %d, output:\n%s", code, err, f.readinessCalls, h.out)
	}
}

// AS-47: not-ready prints one line, exits 1, and does not stop the target.
func TestStartReportsNotReadyAS47(t *testing.T) {
	f := newReadinessFake()
	f.startErr = &containerkind.NotReadyError{Timeout: 30 * time.Second}
	h := setupReadiness(t, f, true)

	code, err := Run([]string{"start", "fizzy"})
	if code != 1 || err != nil || f.stopped ||
		!strings.Contains(h.out.String(), "fizzy started but not ready after 30 s.") {
		t.Fatalf("Run = %d, %v; stopped = %v, output:\n%s", code, err, f.stopped, h.out)
	}
	if strings.Contains(h.out.String(), "start requested") {
		t.Fatalf("not-ready also printed the old start message:\n%s", h.out)
	}
}

// AS-48: a failed ready preflight prevents up from running.
func TestStartReadyPreflightFailureSkipsStartAS48(t *testing.T) {
	f := newReadinessFake()
	f.checksOK = false
	h := setupReadiness(t, f, true)

	code, err := Run([]string{"start", "fizzy"})
	if code != 1 || err == nil || f.readinessCalls != 0 || f.started ||
		!strings.Contains(h.out.String(), "ready script executable") {
		t.Fatalf("Run = %d, %v; started = %v, output:\n%s", code, err, f.started, h.out)
	}
}

// AS-49: an ordinary start failure keeps the existing error path.
func TestStartReadyUpFailureAS49(t *testing.T) {
	f := newReadinessFake()
	f.startErr = errors.New("up failed")
	setupReadiness(t, f, true)

	code, err := Run([]string{"start", "fizzy"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "up failed") {
		t.Fatalf("Run = %d, %v", code, err)
	}
}

// AS-50: no ready key keeps the original message and exit code.
func TestStartWithoutReadyOutputUnchangedAS50(t *testing.T) {
	f := newReadinessFake()
	h := setupReadiness(t, f, false)

	code, err := Run([]string{"start", "fizzy"})
	if code != 0 || err != nil || f.readinessCalls != 0 ||
		!strings.Contains(h.out.String(), "fizzy start requested.") {
		t.Fatalf("Run = %d, %v; calls = %d, output:\n%s", code, err, f.readinessCalls, h.out)
	}
}

// AS-52: list and dashboard snapshots do not invoke readiness.
func TestListAndDashboardDoNotRunReadyAS52(t *testing.T) {
	f := newReadinessFake()
	setupReadiness(t, f, true)
	if code, err := Run([]string{"list"}); code != 0 || err != nil {
		t.Fatalf("list = %d, %v", code, err)
	}

	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	(&controller{cfg: cfg, entries: entries}).Snapshot()
	if f.readinessCalls != 0 {
		t.Fatalf("readiness calls = %d, want zero", f.readinessCalls)
	}
}

func TestDashboardStartUsesReadinessResult(t *testing.T) {
	f := newReadinessFake()
	f.elapsed = 2 * time.Second
	setupReadiness(t, f, true)
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)

	ok, out := (&controller{cfg: cfg, entries: entries}).Run(ui.Start, "fizzy")
	if !ok || f.readinessCalls != 1 || !strings.Contains(out, "fizzy started and ready (2 s).") {
		t.Fatalf("dashboard start = %v, calls = %d, output:\n%s", ok, f.readinessCalls, out)
	}
}
