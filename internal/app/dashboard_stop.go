package app

import (
	"context"

	"github.com/platform41/dockguard/internal/ui"
)

// dashboardStop bridges the shared stop flow to one dashboard question at a time.
// The worker can finish after cancellation even when the dashboard has exited.
type dashboardStop struct {
	ctx       context.Context
	questions chan ui.StopQuestion
	answers   chan bool
	result    chan ui.StopUpdate
}

func (c *controller) BeginStop(ctx context.Context, action ui.Action, name string, approval *ui.Confirmation) ui.StopSequence {
	stop := &dashboardStop{
		ctx: ctx, questions: make(chan ui.StopQuestion), answers: make(chan bool),
		result: make(chan ui.StopUpdate, 1),
	}
	go func() {
		prompts := &stopPrompts{ctx: ctx, ask: stop.ask, checked: true}
		ok, output := c.run(action, name, approval, prompts)
		stop.result <- ui.StopUpdate{OK: ok, Output: output}
	}()
	return stop
}

func (s *dashboardStop) ask(text string, defaultYes bool) bool {
	question := ui.StopQuestion{Text: text, DefaultYes: defaultYes}
	select {
	case <-s.ctx.Done():
		return false
	case s.questions <- question:
	}
	select {
	case <-s.ctx.Done():
		return false
	case yes := <-s.answers:
		return yes && s.ctx.Err() == nil
	}
}

func (s *dashboardStop) Next() ui.StopUpdate {
	select {
	case question := <-s.questions:
		return ui.StopUpdate{Question: &question}
	case result := <-s.result:
		return result
	}
}

func (s *dashboardStop) Step(yes bool) ui.StopUpdate {
	select {
	case <-s.ctx.Done():
	case s.answers <- yes:
	}
	return s.Next()
}
