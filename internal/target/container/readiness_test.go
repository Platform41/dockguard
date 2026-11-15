package container

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/config"
)

type readinessClock struct {
	current time.Time
}

func installReadinessClock(t *testing.T) *readinessClock {
	t.Helper()
	clock := &readinessClock{current: time.Now()}
	origNow, origSleep, origPoll, origRun := now, sleep, readyPollInterval, runDetachedContext
	now = func() time.Time { return clock.current }
	sleep = func(d time.Duration) { clock.current = clock.current.Add(d) }
	readyPollInterval = time.Second
	t.Cleanup(func() {
		now, sleep, readyPollInterval, runDetachedContext = origNow, origSleep, origPoll, origRun
	})
	return clock
}

func readinessTarget(timeoutSeconds int) *Target {
	tg := fizzy(false)
	tg.cfg.Container.Ready = "/x/fizzy-ready.sh"
	tg.cfg.Container.ReadyTimeoutSeconds = timeoutSeconds
	return tg
}

// AS-46: readiness is polled once per second until it succeeds.
func TestStartReadyOnLaterAttemptAS46(t *testing.T) {
	m := &machine{}
	install(t, m)
	installReadinessClock(t)
	attempts := 0
	runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
		m.ran = append(m.ran, name)
		attempts++
		if attempts < 3 {
			return "", 1, nil
		}
		return "", 0, nil
	}

	elapsed, err := readinessTarget(30).StartWithReadiness()
	if err != nil || attempts != 3 || elapsed != 2*time.Second {
		t.Fatalf("StartWithReadiness = %v, %v; attempts = %d, want 2s after three attempts", elapsed, err, attempts)
	}
}

// AS-47: a target that never becomes ready returns a typed error and is not stopped.
func TestStartNotReadyLeavesContainerRunningAS47(t *testing.T) {
	m := &machine{}
	install(t, m)
	installReadinessClock(t)
	attempts := 0
	runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
		m.ran = append(m.ran, name)
		attempts++
		return "", 1, nil
	}

	_, err := readinessTarget(3).StartWithReadiness()
	var notReady *NotReadyError
	if !errors.As(err, &notReady) || notReady.Timeout != 3*time.Second {
		t.Fatalf("StartWithReadiness error = %#v, want 3s NotReadyError", err)
	}
	if attempts != 3 || ran(m, "/x/fizzy-down.sh") {
		t.Fatalf("attempts = %d, ran = %v; down must not run", attempts, m.ran)
	}
}

// AS-49: an up failure short-circuits readiness.
func TestStartUpFailureSkipsReadyAS49(t *testing.T) {
	m := &machine{scriptCode: 1}
	install(t, m)
	installReadinessClock(t)
	runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
		t.Fatal("ready ran after up failed")
		return "", 0, nil
	}

	_, err := readinessTarget(30).StartWithReadiness()
	var notReady *NotReadyError
	if err == nil || errors.As(err, &notReady) || ran(m, "/x/fizzy-ready.sh") {
		t.Fatalf("StartWithReadiness error = %v, ran = %v", err, m.ran)
	}
}

// AS-50: no ready key runs only up and preserves the existing result.
func TestStartWithoutReadyAS50(t *testing.T) {
	m := &machine{}
	install(t, m)
	installReadinessClock(t)
	runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
		t.Fatal("context runner called without ready")
		return "", 0, nil
	}

	elapsed, err := fizzy(false).StartWithReadiness()
	if err != nil || elapsed != 0 || !ran(m, "/x/fizzy-up.sh") {
		t.Fatalf("StartWithReadiness = %v, %v; ran = %v", elapsed, err, m.ran)
	}
}

// AS-57: hanging attempts are capped at 5s and the overall deadline wins.
func TestStartReadyAttemptCapAndOverallDeadlineAS57(t *testing.T) {
	for _, tt := range []struct {
		name        string
		timeout     int
		wantBounds  []time.Duration
		wantElapsed time.Duration
	}{
		{"attempt cap", 12, []time.Duration{5 * time.Second, 5 * time.Second}, 12 * time.Second},
		{"overall deadline shorter than cap", 3, []time.Duration{3 * time.Second}, 3 * time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &machine{}
			install(t, m)
			clock := installReadinessClock(t)
			started := clock.current
			var bounds []time.Duration
			runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
				deadline, ok := ctx.Deadline()
				if !ok {
					t.Fatal("ready attempt has no deadline")
				}
				bounds = append(bounds, deadline.Sub(clock.current))
				clock.current = deadline
				return "", -1, context.DeadlineExceeded
			}

			_, err := readinessTarget(tt.timeout).StartWithReadiness()
			var notReady *NotReadyError
			if !errors.As(err, &notReady) || clock.current.Sub(started) != tt.wantElapsed {
				t.Fatalf("error = %v, elapsed = %v; want %v", err, clock.current.Sub(started), tt.wantElapsed)
			}
			if len(bounds) != len(tt.wantBounds) {
				t.Fatalf("attempt bounds = %v, want %v", bounds, tt.wantBounds)
			}
			for i := range bounds {
				if bounds[i] != tt.wantBounds[i] {
					t.Fatalf("attempt bounds = %v, want %v", bounds, tt.wantBounds)
				}
			}
		})
	}
}

// AS-48: ready has its own executable preflight check only when configured.
func TestReadyExecutableCheckAS48(t *testing.T) {
	install(t, &machine{})
	scripts := t.TempDir()
	up := filepath.Join(scripts, "up.sh")
	down := filepath.Join(scripts, "down.sh")
	ready := filepath.Join(scripts, "ready.sh")
	for path, mode := range map[string]os.FileMode{up: 0o755, down: 0o755, ready: 0o644} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	tg := New(config.Target{Name: "fizzy", Container: &config.Container{
		Up: up, Down: down, Ready: ready, Containers: []string{"fizzy-app"},
	}}, "/Volumes/X")

	found := false
	for _, item := range tg.Checks() {
		if item.Name == "ready script executable" {
			found = true
			if item.OK {
				t.Fatal("non-executable ready script passed")
			}
		}
	}
	if !found {
		t.Fatal("ready script executable check is missing")
	}

	tg.cfg.Container.Ready = ""
	for _, item := range tg.Checks() {
		if item.Name == "ready script executable" {
			t.Fatal("ready check added without a ready script")
		}
	}
}

// AS-52: state probes used by list and dashboard refresh never run ready.
func TestStateDoesNotRunReadyAS52(t *testing.T) {
	m := &machine{systemUp: false}
	install(t, m)
	installReadinessClock(t)
	runDetachedContext = func(ctx context.Context, name string, args ...string) (string, int, error) {
		t.Fatal("ready ran during a state probe")
		return "", 0, nil
	}
	if _, err := readinessTarget(30).State(); err != nil {
		t.Fatal(err)
	}
}
