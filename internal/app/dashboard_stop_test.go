package app

import (
	"context"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/holders"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
)

func dashboardHolderSetup(t *testing.T, targets ...*fake) (*harness, *controller) {
	t.Helper()
	h := setup(t, true, false, targets...)
	holderCLI(t, false, "")
	dependencyTerminal = func() bool {
		t.Error("dashboard used the CLI terminal check")
		return false
	}
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	return h, &controller{cfg: cfg, entries: entries}
}

func startDashboardStop(t *testing.T, c *controller, action ui.Action, name string, approval *ui.Confirmation) (ui.StopSequence, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return c.BeginStop(ctx, action, name, approval), cancel
}

func dashboardQuestion(t *testing.T, got ui.StopUpdate, text string, defaultYes bool) {
	t.Helper()
	if got.Question == nil || !strings.Contains(got.Question.Text, text) || got.Question.DefaultYes != defaultYes {
		t.Fatalf("question = %+v, result = %+v, want %q with default %v", got.Question, got, text, defaultYes)
	}
}

func withDashboardSignal(fn func(pid int, sig syscall.Signal) error) {
	factory := holderService
	holderService = func() holders.Service {
		s := factory()
		s.Signal = fn
		return s
	}
}

func TestDashboardClearsHoldersAS147AS149(t *testing.T) {
	for _, action := range []ui.Action{ui.StopAllEject, ui.StopProcesses} {
		t.Run(action.String(), func(t *testing.T) {
			f := &fake{name: "myapp", states: states(target.Running, target.Stopped)}
			h, c := dashboardHolderSetup(t, f)
			holding, scans := true, 0
			holdersOf = func(string) ([]platform.Holder, error) {
				scans++
				if action == ui.StopAllEject && !f.stopped {
					t.Error("holder scan ran before target stops")
				}
				if holding {
					return []platform.Holder{editor}, nil
				}
				return nil, nil
			}
			var signals []syscall.Signal
			withDashboardSignal(func(pid int, sig syscall.Signal) error {
				if pid != editor.PID {
					t.Errorf("signal for PID %d", pid)
				}
				if sig == 0 {
					return syscall.ESRCH
				}
				signals = append(signals, sig)
				holding = false
				return nil
			})

			sequence, _ := startDashboardStop(t, c, action, "", nil)
			dashboardQuestion(t, sequence.Next(), "Quit it?", true)
			result := sequence.Step(true)
			wantEject := action == ui.StopAllEject
			wantScans := 3
			if wantEject {
				wantScans++
			}
			if !result.OK || result.Question != nil || h.ejected != wantEject || f.stopped != wantEject ||
				!slices.Equal(signals, []syscall.Signal{syscall.SIGTERM}) || scans != wantScans ||
				!strings.Contains(result.Output, "editor (PID 42): quit") {
				t.Fatalf("result %+v eject=%v stopped=%v signals=%v scans=%d", result, h.ejected, f.stopped, signals, scans)
			}
			if snap := c.Snapshot(); len(snap.Holders) != 0 {
				t.Fatalf("holders after refresh: %v", snap.Holders)
			}
		})
	}
}

func TestDashboardEjectRechecksTargetsAfterHolderAnswer(t *testing.T) {
	f := &fake{name: "myapp", states: states(target.Stopped, target.Stopped, target.Running)}
	h, c := dashboardHolderSetup(t, f)
	holding := true
	holdersOf = func(string) ([]platform.Holder, error) {
		if holding {
			return []platform.Holder{editor}, nil
		}
		return nil, nil
	}
	withDashboardSignal(func(_ int, sig syscall.Signal) error {
		if sig == 0 {
			return syscall.ESRCH
		}
		holding = false
		return nil
	})
	seq, _ := startDashboardStop(t, c, ui.StopAllEject, "", nil)
	dashboardQuestion(t, seq.Next(), "Quit it?", true)
	result := seq.Step(true)
	if h.ejected || result.OK || !strings.Contains(result.Output, "myapp: running") {
		t.Fatalf("eject=%v result=%+v", h.ejected, result)
	}
}

func TestDashboardCancelsBeforeAnswerAS148(t *testing.T) {
	h, c := dashboardHolderSetup(t)
	holdersOf = func(string) ([]platform.Holder, error) { return []platform.Holder{editor}, nil }
	seq, cancel := startDashboardStop(t, c, ui.StopAllEject, "", nil)
	dashboardQuestion(t, seq.Next(), "Quit it?", true)
	cancel()
	result := seq.Next()
	if result.OK || result.Question != nil || h.ejected || !strings.Contains(result.Output, "Stop cancelled.") {
		t.Fatalf("eject=%v result=%+v", h.ejected, result)
	}
}

func TestDashboardFinishesQuitWaitAfterCancellationAS151(t *testing.T) {
	h, c := dashboardHolderSetup(t)
	other := editor
	other.PID++
	holdersOf = func(string) ([]platform.Holder, error) { return []platform.Holder{other, editor}, nil }
	var sent []syscall.Signal
	withDashboardSignal(func(pid int, sig syscall.Signal) error {
		if pid != editor.PID {
			t.Errorf("signal for unanswered holder PID %d", pid)
		}
		if sig != 0 {
			sent = append(sent, sig)
		}
		return nil
	})

	entered, resume := make(chan struct{}), make(chan struct{})
	advance := sleep
	var waited time.Duration
	sleep = func(d time.Duration) {
		if waited == 0 {
			close(entered)
			<-resume
		}
		waited += d
		advance(d)
	}
	seq, cancel := startDashboardStop(t, c, ui.StopAllEject, "", nil)
	dashboardQuestion(t, seq.Next(), "Quit it?", true)
	done := make(chan ui.StopUpdate, 1)
	go func() { done <- seq.Step(true) }()
	<-entered
	cancel()
	close(resume)
	result := <-done

	if result.OK || result.Question != nil || h.ejected || waited != 5*time.Second ||
		!slices.Equal(sent, []syscall.Signal{syscall.SIGTERM}) ||
		!strings.Contains(result.Output, "editor (PID 42): still using the drive") ||
		!strings.Contains(result.Output, "Stop cancelled.") {
		t.Fatalf("eject=%v waited=%v signals=%v result=%+v", h.ejected, waited, sent, result)
	}
}

func TestDashboardStopsUnmanagedContainerAS153(t *testing.T) {
	for _, tt := range []struct {
		action ui.Action
		yes    bool
	}{
		{ui.StopAllEject, true}, {ui.StopAllEject, false}, {ui.Stop, true},
	} {
		name := tt.action.String() + "/decline"
		if tt.yes {
			name = tt.action.String() + "/accept"
		}
		t.Run(name, func(t *testing.T) {
			h, c := dashboardHolderSetup(t)
			tc := config.Target{Name: "container", Kind: config.KindContainerSystem}
			system := &fakeUnmanagedSystem{fake: &fake{name: tc.Name, kind: tc.Kind}, names: []string{"buildkit"}}
			c.entries = []entry{{t: system, cfg: tc}}
			name := ""
			if tt.action == ui.Stop {
				name = tc.Name
			}
			seq, _ := startDashboardStop(t, c, tt.action, name, nil)
			dashboardQuestion(t, seq.Next(), "Container buildkit is running", true)
			result := seq.Step(tt.yes)
			if result.OK != tt.yes || result.Question != nil || system.stopped != tt.yes ||
				h.ejected != (tt.yes && tt.action == ui.StopAllEject) ||
				strings.Contains(result.Output, "container start buildkit") != tt.yes {
				t.Fatalf("eject=%v stopped=%v result=%+v", h.ejected, system.stopped, result)
			}
			if tt.yes && !slices.Equal(system.requested, []string{"buildkit"}) || !tt.yes && len(system.requested) != 0 {
				t.Fatalf("requested = %v", system.requested)
			}
		})
	}
}
