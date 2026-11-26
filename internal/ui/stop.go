package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// StopQuestion is the next consent required by a stop action in execution order.
type StopQuestion struct {
	Text       string
	DefaultYes bool
}

// StopUpdate contains the next question, or the final action result.
type StopUpdate struct {
	Question *StopQuestion
	OK       bool
	Output   string
}

// StopSequence lists fresh blocker questions as the action reaches them. Next
// reads the first question or result, and Step applies one answer before reading
// the following question or result. The model calls them serially in commands.
type StopSequence interface {
	Next() StopUpdate
	Step(yes bool) StopUpdate
}

type stopQuestionMsg struct {
	pending  pending
	sequence StopSequence
	question StopQuestion
}

func stopMessage(p pending, sequence StopSequence, update StopUpdate) tea.Msg {
	if update.Question != nil {
		return stopQuestionMsg{pending: p, sequence: sequence, question: *update.Question}
	}
	return actionMsg{action: p.action, target: p.target, ok: update.OK, output: update.Output}
}

type activeStop struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (m Model) canStopProcesses() bool { return m.snap.Mounted && len(m.snap.Holders) > 0 }

func (m Model) stopKey(key string) (tea.Model, tea.Cmd) {
	if key == "esc" || key == "q" || key == "ctrl+c" {
		m.stopping.cancel()
		m.quitAfterStop = m.quitAfterStop || key != "esc"
		if q := m.stopQuestion; q != nil {
			m.stopQuestion = nil
			return m, func() tea.Msg { return stopMessage(q.pending, q.sequence, q.sequence.Next()) }
		}
		return m, nil
	}

	q := m.stopQuestion
	if q == nil {
		return m, nil
	}
	var yes bool
	switch key {
	case "y", "Y":
		yes = true
	case "n", "N":
		yes = false
	case "enter":
		yes = q.question.DefaultYes
	default:
		return m, nil
	}
	m.stopQuestion = nil
	return m, func() tea.Msg { return stopMessage(q.pending, q.sequence, q.sequence.Step(yes)) }
}
