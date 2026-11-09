package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/platform41/dockguard/internal/target"
)

type call struct {
	action Action
	target string
}

type fakeCtl struct {
	snap   Snapshot
	calls  []call
	ok     bool
	output string
	snaps  int
}

func (f *fakeCtl) Snapshot() Snapshot { f.snaps++; return f.snap }
func (f *fakeCtl) Run(a Action, t string) (bool, string) {
	f.calls = append(f.calls, call{a, t})
	return f.ok, f.output
}

func snapshot(states ...target.State) Snapshot {
	names := []string{"docker", "xcode", "myapp-db", "fizzy"}
	s := Snapshot{Drive: "/Volumes/ExternalSSD", Mounted: true, UsedPct: 30, FreeGB: 685, TotalGB: 1000}
	for i, st := range states {
		s.Rows = append(s.Rows, Row{Name: names[i], Kind: "kind", Path: "/p/" + names[i], State: st})
	}
	return s
}

// loaded returns a model that has received its first snapshot.
func loaded(f *fakeCtl) Model {
	m, _ := New(f).Update(snapshotMsg(f.snap))
	return m.(Model)
}

func press(m Model, keys ...string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

// AS-31: the drive, the eject line and every target with its state.
func TestViewShowsDriveEjectLineAndTargets(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped, target.Running, target.Running, target.Partial)}
	view := loaded(f).View()
	for _, want := range []string{"DockGuard · ExternalSSD", "/Volumes/ExternalSSD", "mounted", "685 GB free",
		"docker", "xcode", "myapp-db", "fizzy", "running", "partial", "stopped"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

// AS-32: safe to eject only when every target is confirmed stopped.
func TestEjectLine(t *testing.T) {
	stopped := loaded(&fakeCtl{snap: snapshot(target.Stopped, target.Stopped)}).View()
	if !strings.Contains(stopped, "safe to eject") {
		t.Fatalf("all stopped:\n%s", stopped)
	}

	f := &fakeCtl{snap: snapshot(target.Stopped, target.Running, target.Stopped, target.Unknown)}
	f.snap.Rows[3].Err = "container system status: exit 1, status \"registered\""
	blocked := loaded(f).View()
	if strings.Contains(blocked, "safe to eject") || !strings.Contains(blocked, "blocked by:") ||
		!strings.Contains(blocked, "xcode (running)") || !strings.Contains(blocked, "fizzy (unknown)") {
		t.Fatalf("blocked:\n%s", blocked)
	}

	unmounted := &fakeCtl{snap: Snapshot{Drive: "/Volumes/ExternalSSD", Rows: []Row{{Name: "docker", Kind: "k"}}}}
	if v := loaded(unmounted).View(); !strings.Contains(v, "not mounted") || !strings.Contains(v, "nothing to eject") {
		t.Fatalf("unmounted:\n%s", v)
	}
}

func TestUnknownStateShowsTheProbeErrorForTheSelectedTarget(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Unknown)}
	f.snap.Rows[0].Err = "docker desktop status not supported"
	if v := loaded(f).View(); !strings.Contains(v, "docker desktop status not supported") {
		t.Fatalf("view:\n%s", v)
	}
}

// AS-33: x and E do nothing until confirmed; any other key cancels.
func TestStopAndEjectNeedConfirmation(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running, target.Running), ok: true}
	m := loaded(f)

	m, cmd := press(m, "down", "x")
	if cmd != nil || m.confirming == nil || !strings.Contains(m.View(), "Stop xcode?") {
		t.Fatalf("x should ask first:\n%s", m.View())
	}
	m, cmd = press(m, "n")
	if cmd != nil || m.confirming != nil || len(f.calls) != 0 {
		t.Fatalf("n should cancel; calls %v", f.calls)
	}

	m, _ = press(m, "E")
	if !strings.Contains(m.View(), "Stop every target and eject /Volumes/ExternalSSD?") {
		t.Fatalf("E should ask first:\n%s", m.View())
	}
	m, cmd = press(m, "y")
	if cmd == nil || !m.busy {
		t.Fatal("y should run the action")
	}
	cmd()
	if len(f.calls) != 1 || f.calls[0] != (call{StopAllEject, ""}) {
		t.Fatalf("calls = %v", f.calls)
	}
}

func TestStartAndCheckRunOnTheSelectedTarget(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped, target.Stopped, target.Stopped), ok: true}
	m := loaded(f)

	m, cmd := press(m, "down", "down", "s")
	cmd()
	m.busy = false
	_, cmd = press(m, "c")
	cmd()
	want := []call{{Start, "myapp-db"}, {Check, "myapp-db"}}
	if len(f.calls) != 2 || f.calls[0] != want[0] || f.calls[1] != want[1] {
		t.Fatalf("calls = %v", f.calls)
	}
}

// AS-34: a refused eject shows why, as the CLI would.
func TestRefusedEjectIsShown(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running), ok: false,
		output: "Not ejecting /Volumes/ExternalSSD: these targets aren't confirmed stopped:\n- xcode: running\ndrive not ejected\n"}
	m := loaded(f)
	m, cmd := press(m, "E", "y")
	next, _ := m.Update(cmd())
	view := next.(Model).View()
	for _, want := range []string{"✗", "stop --all --eject", "Not ejecting /Volumes/ExternalSSD", "- xcode: running"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestActionsAreIgnoredWhileBusy(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped), ok: true}
	m := loaded(f)
	m.busy = true
	if _, cmd := press(m, "s"); cmd != nil {
		t.Fatal("start ran while another action was busy")
	}
}

// runAll runs a command, expanding batches, and returns the messages it produced.
func runAll(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runAll(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestRefreshNeverOverlaps(t *testing.T) {
	orig := RefreshInterval
	t.Cleanup(func() { RefreshInterval = orig })
	RefreshInterval = time.Millisecond

	f := &fakeCtl{snap: snapshot(target.Stopped)}
	m := loaded(f)

	// A tick while a probe is in flight schedules only the next tick: no new probe.
	m.refreshing = true
	_, cmd := m.Update(tickMsg{})
	before := f.snaps
	runAll(cmd)
	if f.snaps != before {
		t.Fatalf("probed %d times while a probe was running", f.snaps-before)
	}

	// The same while an action runs.
	m.refreshing, m.busy = false, true
	_, cmd = m.Update(tickMsg{})
	runAll(cmd)
	if f.snaps != before {
		t.Fatal("probed while an action was running")
	}

	// Otherwise a tick probes once.
	m.busy = false
	_, cmd = m.Update(tickMsg{})
	runAll(cmd)
	if f.snaps != before+1 {
		t.Fatalf("probes = %d, want 1", f.snaps-before)
	}
}

func TestQuit(t *testing.T) {
	m := loaded(&fakeCtl{snap: snapshot(target.Stopped)})
	if _, cmd := press(m, "q"); cmd == nil {
		t.Fatal("q should quit")
	}
}

func TestRowsStayInsideTheTerminalWidth(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped, target.Running)}
	f.snap.Rows[0].Path = "/Volumes/ExternalSSD/a/very/long/path/that/would/never/fit/in/eighty/columns/DockerDesktop"
	m := loaded(f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, line := range strings.Split(next.(Model).View(), "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("line is %d wide:\n%s", w, line)
		}
	}
	if !strings.Contains(next.(Model).View(), "…") || !strings.Contains(next.(Model).View(), "DockerDesktop") {
		t.Fatal("long path should be cut from the left, keeping its end")
	}
}

func TestColumnsLineUp(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped, target.Running)}
	f.snap.Rows[0].Kind, f.snap.Rows[1].Kind = "docker-desktop", "app"
	var starts []int
	for _, line := range strings.Split(loaded(f).View(), "\n") {
		plain := ansi.Strip(line)
		if i := strings.Index(plain, "/p/"); i >= 0 {
			starts = append(starts, lipgloss.Width(plain[:i]))
		}
	}
	if len(starts) != 2 || starts[0] != starts[1] {
		t.Fatalf("path columns start at %v", starts)
	}
}

func TestHeaderBeforeFirstProbe(t *testing.T) {
	if v := New(&fakeCtl{}).View(); strings.Contains(v, "· drive") || !strings.Contains(v, "DockGuard") {
		t.Fatalf("view:\n%s", v)
	}
}

// Live check: a burst such as "jj" arrives as one message and must move twice.
func TestKeyBurstIsHandledKeyByKey(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Stopped, target.Stopped, target.Stopped), ok: true}
	m := loaded(f)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjc")})
	if next.(Model).selected != 2 {
		t.Fatalf("selected = %d, want 2", next.(Model).selected)
	}
	runAll(cmd)
	if len(f.calls) != 1 || f.calls[0] != (call{Check, "myapp-db"}) {
		t.Fatalf("calls = %v", f.calls)
	}
}

func TestConfirmPromptFitsEightyColumns(t *testing.T) {
	m := loaded(&fakeCtl{snap: snapshot(target.Running)})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	m, _ = press(m, "E")
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "eject") && lipgloss.Width(line) > 80 {
			t.Fatalf("prompt is %d wide: %s", lipgloss.Width(line), line)
		}
	}
}

// A pasted "Eye" opens the eject prompt but can't answer it.
func TestBurstCannotConfirm(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running), ok: true}
	m := loaded(f)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Eye")})
	runAll(cmd)
	if len(f.calls) != 0 || next.(Model).confirming == nil {
		t.Fatalf("calls = %v, confirming = %v; the prompt should still be waiting", f.calls, next.(Model).confirming)
	}
}

// Review on PR #14: an action requested while a probe is in flight waits for that probe, so a
// probe and an action never overlap and an older snapshot can't overwrite a newer one.
func TestActionsWaitForAnInFlightRefresh(t *testing.T) {
	orig := RefreshInterval
	t.Cleanup(func() { RefreshInterval = orig })
	RefreshInterval = time.Millisecond

	for _, keys := range [][]string{{"s"}, {"x", "y"}, {"E", "y"}} {
		f := &fakeCtl{snap: snapshot(target.Stopped, target.Stopped), ok: true}
		m := loaded(f)
		m.refreshing = true

		m, cmd := press(m, keys...)
		runAll(cmd)
		if len(f.calls) != 0 {
			t.Fatalf("%v: action ran during a refresh: %v", keys, f.calls)
		}
		if !m.busy || !strings.Contains(m.View(), "Waiting") {
			t.Fatalf("%v: should show the action waiting:\n%s", keys, m.View())
		}

		// While it waits, a tick starts no probe.
		before := f.snaps
		_, cmd = m.Update(tickMsg{})
		runAll(cmd)
		if f.snaps != before {
			t.Fatalf("%v: probed while an action was waiting", keys)
		}

		// The probe's result arrives: now, and only now, the action runs.
		next, cmd := m.Update(snapshotMsg(f.snap))
		m = next.(Model)
		if m.refreshing {
			t.Fatalf("%v: a probe was started alongside the action", keys)
		}
		msgs := runAll(cmd)
		if len(f.calls) != 1 || len(msgs) != 1 {
			t.Fatalf("%v: calls after the refresh = %v, msgs = %v", keys, f.calls, msgs)
		}

		// When the action finishes, exactly one probe follows, after it.
		before = f.snaps
		next, cmd = m.Update(msgs[0])
		m = next.(Model)
		runAll(cmd)
		if f.snaps != before+1 || m.busy {
			t.Fatalf("%v: probes after the action = %d, busy = %v", keys, f.snaps-before, m.busy)
		}
	}
}

// Review on PR #14: the blocker line wraps instead of running past the terminal edge, and every
// blocker stays visible.
func TestBlockerLineWraps(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running, target.Running, target.Running, target.Running)}
	f.snap.Rows[0].Name = "docker-desktop-main"
	f.snap.Rows[3].Name = "myapp-container-stack"
	m := loaded(f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := next.(Model).View()
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("line is %d wide:\n%s", w, line)
		}
	}
	for _, r := range f.snap.Rows {
		if !strings.Contains(ansi.Strip(view), r.Name+" (running)") {
			t.Fatalf("blocker %s missing:\n%s", r.Name, view)
		}
	}
}
