package app

import (
	"fmt"
	"strings"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/target"
)

// stopOrder chooses the last configured ready target each time. Dependencies constrain
// readiness; independent targets retain reverse configuration priority.
func (g dependencyGraph) stopOrder() []entry {
	entries := g.configured
	dependents := map[string]int{}
	for _, e := range entries {
		for _, dep := range e.cfg.DependsOn {
			dependents[dep]++
		}
	}
	done := map[string]bool{}
	order := make([]entry, 0, len(entries))
	for len(order) < len(entries) {
		for i := len(entries) - 1; i >= 0; i-- {
			e := entries[i]
			name := e.t.Name()
			if done[name] || dependents[name] != 0 {
				continue
			}
			order = append(order, e)
			done[name] = true
			for _, dep := range e.cfg.DependsOn {
				dependents[dep]--
			}
			break
		}
	}
	return order
}

func runStopAll(cfg config.Config, entries []entry, eject bool, prompts *stopPrompts) (int, error) {
	hasDeps, hasSystem := false, false
	for _, e := range entries {
		hasDeps = hasDeps || len(e.cfg.DependsOn) > 0
		hasSystem = hasSystem || e.cfg.Kind == config.KindContainerSystem
	}
	if !hasDeps && !hasSystem {
		return runStopAllLegacy(cfg, entries, eject, prompts)
	}
	g, err := newDependencyGraph(entries)
	if err != nil {
		return 1, err
	}
	justStopped := map[string]bool{}
	var failed []string
	// protected maps a prerequisite to the failed descendants that still need it.
	protected := map[string][]string{}
	for _, e := range g.stopOrder() {
		name := e.t.Name()
		if blockers := protected[name]; len(blockers) > 0 {
			output.PrintStopFailed(display(e), fmt.Errorf("still required by targets that failed to stop: %s", strings.Join(blockers, ", ")))
			failed = append(failed, name)
			continue
		}
		if state, _ := e.t.State(); state == target.Stopped {
			output.PrintAlreadyStopped(display(e))
			continue
		}
		stop := func() (bool, error) { return stopWithUnmanagedContainers(e, prompts) }
		probe := e.t.State
		if !hasSystem {
			if legacy, ok := e.t.(sharedSystemStopper); ok {
				stop = legacy.StopStack
				// A legacy stack's aggregate State stays partial until the second pass.
				if own, ok := e.t.(guardStater); ok {
					probe = own.GuardState
				}
			}
		}
		requested, stopErr := stop()
		if stopErr == nil {
			if requested {
				output.PrintStopped(display(e))
				justStopped[name] = true
			} else {
				output.PrintAlreadyStopped(display(e))
			}
			state, probeErr := probe()
			// Confirm the stack itself, not its optional legacy system service.
			deadline := now().Add(stopConfirmTimeout)
			for state != target.Stopped && now().Before(deadline) {
				sleep(stopConfirmPoll)
				state, probeErr = probe()
			}
			if state != target.Stopped {
				stopErr = fmt.Errorf("not confirmed stopped (%s)", state)
				if probeErr != nil {
					stopErr = fmt.Errorf("%w: %v", stopErr, probeErr)
				}
			}
		}
		if stopErr != nil {
			output.PrintStopFailed(display(e), stopErr)
			failed = append(failed, name)
			for dep := range g.needs(name) {
				protected[dep] = append(protected[dep], name)
			}
		}
	}
	if !hasSystem {
		failed = stopLegacySystems(entries, justStopped, failed)
	}
	return finishStopAll(cfg, entries, eject, justStopped, failed, prompts)
}
