package ui

import (
	"context"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type scriptedStop struct {
	ctx       context.Context
	questions []StopQuestion
	answers   []bool
	wait      func()
	finished  bool
	ejected   bool
	action    Action
}

func (s *scriptedStop) Next() StopUpdate {
	if s.ctx.Err() != nil {
		s.finished = true
		return StopUpdate{Output: "Stop cancelled."}
	}
	if len(s.answers) == len(s.questions) {
		s.finished = true
		s.ejected = s.action == StopAllEject
		return StopUpdate{OK: true, Output: "complete"}
	}
	return StopUpdate{Question: &s.questions[len(s.answers)]}
}

func (s *scriptedStop) Step(yes bool) StopUpdate {
	s.answers = append(s.answers, yes)
	if s.wait != nil {
		s.wait()
	}
	return s.Next()
}

type stopTestController struct {
	*fakeCtl
	stop scriptedStop
}

func (c *stopTestController) BeginStop(ctx context.Context, a Action, _ string, _ *Confirmation) StopSequence {
	c.stop.ctx, c.stop.action = ctx, a
	return &c.stop
}

func stopTestModel() (Model, *stopTestController) {
	f := &stopTestController{fakeCtl: &fakeCtl{snap: snapshot()}}
	f.snap.Holders = []Holder{{Name: "editor", PID: 42, Class: "stoppable"}}
	next, _ := New(f).Update(snapshotMsg(f.snap))
	return next.(Model), f
}

func stopKey(m Model, key tea.KeyType) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: key})
	return next.(Model), cmd
}

func runStopCommand(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	next, more := m.Update(cmd())
	return next.(Model), more
}

func TestDashboardStopQuestionsAS147AS149AS153(t *testing.T) {
	for _, start := range []string{"E", "p"} {
		t.Run(start, func(t *testing.T) {
			m, f := stopTestModel()
			questions := []StopQuestion{{Text: "Quit editor?", DefaultYes: true}, {Text: "Force-quit? Unsaved work will be lost."}}
			if start == "E" {
				questions = append([]StopQuestion{{Text: "Stop buildkit?", DefaultYes: true}}, questions...)
			}
			f.stop.questions = questions
			m, cmd := press(m, start)
			if start == "E" {
				m, cmd = press(m, "y")
			}
			m, cmd = runStopCommand(t, m, cmd)
			for i, q := range questions {
				suffix := "[y/N]"
				if q.DefaultYes {
					suffix = "[Y/n]"
				}
				if !m.busy || !strings.Contains(m.View(), q.Text) || !strings.Contains(m.View(), suffix) {
					t.Fatalf("question %d missing in %s", i, m.View())
				}
				// One burst can answer only the visible question.
				if i == len(questions)-1 && start == "E" {
					m, cmd = press(m, "yy")
				} else {
					m, cmd = stopKey(m, tea.KeyEnter)
				}
				m, cmd = runStopCommand(t, m, cmd)
			}
			want := []bool{true, false}
			if start == "E" {
				want = []bool{true, true, true}
			}
			if m.busy || !slices.Equal(f.stop.answers, want) || f.stop.ejected != (start == "E") {
				t.Fatalf("busy %v answers %v eject %v", m.busy, f.stop.answers, f.stop.ejected)
			}
			f.snap.Holders = nil
			m, _ = runStopCommand(t, m, cmd)
			if !strings.Contains(m.View(), "safe to eject") {
				t.Fatal(m.View())
			}
		})
	}
}

func TestDashboardStopCancelsAS148AS151(t *testing.T) {
	for _, duringWait := range []bool{false, true} {
		m, f := stopTestModel()
		f.stop.questions = []StopQuestion{{Text: "Quit editor?", DefaultYes: true}, {Text: "Quit another?"}}
		m, cmd := press(m, "E", "y")
		m, _ = runStopCommand(t, m, cmd)
		if duringWait {
			m, cmd = press(m, "y")
			started, resume, done := make(chan struct{}), make(chan struct{}), make(chan tea.Msg, 1)
			f.stop.wait = func() { close(started); <-resume }
			go func(cmd tea.Cmd) { done <- cmd() }(cmd)
			<-started
			if _, refresh := press(m, "r"); refresh != nil {
				t.Error("refresh started during wait")
			}
			if _, other := press(m, "p"); other != nil {
				t.Error("another action started during wait")
			}
			m, cmd = stopKey(m, tea.KeyEsc)
			if cmd != nil || !m.busy {
				t.Error("cancel interrupted the in-flight step")
			}
			close(resume)
			next, refresh := m.Update(<-done)
			m, cmd = next.(Model), refresh
		} else {
			m, cmd = stopKey(m, tea.KeyEsc)
			m, cmd = runStopCommand(t, m, cmd)
		}
		if m.busy || cmd == nil || !f.stop.finished || f.stop.ejected ||
			len(f.stop.answers) != map[bool]int{false: 0, true: 1}[duringWait] {
			t.Fatalf("busy=%v refresh=%v stop=%+v", m.busy, cmd != nil, f.stop)
		}
	}
}

func TestDashboardProcessKeyVisibilityAS150(t *testing.T) {
	for _, mounted := range []bool{false, true} {
		for _, holding := range []bool{false, true} {
			m, _ := stopTestModel()
			m.snap.Mounted = mounted
			if !holding {
				m.snap.Holders = nil
			}
			_, cmd := press(m, "p")
			want := mounted && holding
			if strings.Contains(m.View(), "p processes") != want || (cmd != nil) != want {
				t.Fatalf("mounted=%v holding=%v cmd=%v\n%s", mounted, holding, cmd != nil, m.View())
			}
			if want && !strings.Contains(m.View(), "[stoppable]") {
				t.Fatal(m.View())
			}
		}
	}
}

func TestDashboardStopQuestionAcceptsOnlyAnswerKeys(t *testing.T) {
	m, f := stopTestModel()
	f.stop.questions = []StopQuestion{{Text: "Quit editor?", DefaultYes: true}}
	m, cmd := press(m, "p")
	m, _ = runStopCommand(t, m, cmd)
	m, cmd = press(m, "r")
	if cmd != nil || !strings.Contains(m.View(), "Quit editor?") {
		t.Fatalf("an unrelated key dismissed the question: %s", m.View())
	}
	m, cmd = press(m, "n")
	m, _ = runStopCommand(t, m, cmd)
	if m.busy || !slices.Equal(f.stop.answers, []bool{false}) {
		t.Fatalf("busy=%v answers=%v", m.busy, f.stop.answers)
	}
}
