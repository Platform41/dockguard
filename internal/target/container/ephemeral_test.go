package container

import (
	"reflect"
	"testing"
)

func ephemeralTarget(stopSystem bool) *Target {
	tg := fizzy(stopSystem)
	tg.cfg.Container.Ephemeral = true
	return tg
}

// AS-54: the callback fires immediately before down, never earlier in the stop path.
func TestDiscardCallbackImmediatelyBeforeDownAS54(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "running", "fizzy-app": "running"}}
	install(t, m)
	tg := ephemeralTarget(false)
	var events []string
	tg.OnDiscard = func(name string) { events = append(events, "discard "+name) }
	originalRunDetached := runDetached
	runDetached = func(name string, args ...string) (string, int, error) {
		events = append(events, "down "+name)
		return originalRunDetached(name, args...)
	}

	stopped, err := tg.StopStack()
	if !stopped || err != nil {
		t.Fatalf("StopStack = %v, %v", stopped, err)
	}
	want := []string{"discard fizzy", "down /x/fizzy-down.sh"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestDiscardCallbackRequiresEphemeral(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "running", "fizzy-app": "running"}}
	install(t, m)
	tg := fizzy(false)
	tg.OnDiscard = func(name string) { t.Fatal("callback ran for a non-ephemeral target") }
	if stopped, err := tg.StopStack(); !stopped || err != nil {
		t.Fatalf("StopStack = %v, %v", stopped, err)
	}
}

// AS-56: a fully stopped target neither runs down nor emits a discard callback.
func TestDiscardCallbackFullyStoppedAS56(t *testing.T) {
	m := &machine{systemUp: false}
	install(t, m)
	tg := ephemeralTarget(true)
	calls := 0
	tg.OnDiscard = func(name string) { calls++ }

	stopped, err := tg.Stop()
	if stopped || err != nil || calls != 0 || ran(m, "/x/fizzy-down.sh") {
		t.Fatalf("Stop = %v, %v; callbacks = %d, ran = %v", stopped, err, calls, m.ran)
	}
}

// AS-58: single-target system-only stop does not run down or emit a callback.
func TestDiscardCallbackSingleSystemOnlyAS58(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "stopped", "fizzy-app": "stopped"}}
	install(t, m)
	tg := ephemeralTarget(true)
	calls := 0
	tg.OnDiscard = func(name string) { calls++ }

	stopped, err := tg.Stop()
	if !stopped || err != nil || calls != 0 || ran(m, "/x/fizzy-down.sh") || !ran(m, "container system stop") {
		t.Fatalf("Stop = %v, %v; callbacks = %d, ran = %v", stopped, err, calls, m.ran)
	}
}

// AS-59: stop --all two-pass system-only stop does not run down or emit a callback.
func TestDiscardCallbackStopAllSystemOnlyAS59(t *testing.T) {
	m := &machine{systemUp: true, states: map[string]string{"fizzy-db": "stopped", "fizzy-app": "stopped"}}
	install(t, m)
	tg := ephemeralTarget(true)
	calls := 0
	tg.OnDiscard = func(name string) { calls++ }

	stackStopped, err := tg.StopStack()
	if stackStopped || err != nil {
		t.Fatalf("StopStack = %v, %v", stackStopped, err)
	}
	systemStopped, err := tg.StopSystem()
	if !systemStopped || err != nil || calls != 0 || ran(m, "/x/fizzy-down.sh") {
		t.Fatalf("StopSystem = %v, %v; callbacks = %d, ran = %v", systemStopped, err, calls, m.ran)
	}
}
