package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/target"
)

var (
	dependencyTerminal            = stdioIsTerminal
	dependencyInput     io.Reader = os.Stdin
	startConfirmTimeout           = 30 * time.Second
	startConfirmPoll              = time.Second
)

// dependencyGraph keeps configuration order for independent targets and visits prerequisites first.
type dependencyGraph struct {
	order  []entry
	byName map[string]entry
}

func newDependencyGraph(entries []entry) (dependencyGraph, error) {
	g := dependencyGraph{byName: map[string]entry{}}
	for _, e := range entries {
		g.byName[e.t.Name()] = e
	}
	visited := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		e, ok := g.byName[name]
		if !ok {
			return fmt.Errorf("unknown dependency %q", name)
		}
		if visited[name] == 2 {
			return nil
		}
		if visited[name] == 1 {
			return fmt.Errorf("dependency cycle at %q", name)
		}
		visited[name] = 1
		for _, dep := range e.cfg.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[name] = 2
		g.order = append(g.order, e)
		return nil
	}
	for _, e := range entries {
		if err := visit(e.t.Name()); err != nil {
			return g, err
		}
	}
	return g, nil
}

func (g dependencyGraph) needs(name string) map[string]bool {
	result := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		for _, dep := range g.byName[name].cfg.DependsOn {
			if !result[dep] {
				result[dep] = true
				visit(dep)
			}
		}
	}
	visit(name)
	return result
}

// askDependency is CLI-only. Dashboard actions must not compete with the TUI for stdin.
func askDependency(question string) bool {
	if !dependencyTerminal() {
		return false
	}
	fmt.Fprint(output.Out, question+" [y/N] ")
	answer, err := bufio.NewReader(dependencyInput).ReadString('\n')
	fmt.Fprintln(output.Out)
	if err != nil {
		// Incomplete input at EOF is not consent.
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
func declineDependency(string) bool { return false }

func runStart(cfg config.Config, entries []entry, e entry) (int, error) {
	return startWithDependencies(cfg, entries, e, askDependency)
}

func startWithDependencies(cfg config.Config, entries []entry, e entry, confirm func(string) bool) (int, error) {
	if len(e.cfg.DependsOn) == 0 {
		return startTarget(cfg, e)
	}
	g, err := newDependencyGraph(entries)
	if err != nil {
		return 1, err
	}
	needed := g.needs(e.t.Name())
	var missing []entry
	for _, dep := range g.order {
		if !needed[dep.t.Name()] {
			continue
		}
		state, err := dep.t.State()
		if state == target.Unknown {
			return 1, fmt.Errorf("not starting %s: dependency %s state unknown: %s", e.t.Name(), dep.t.Name(), errMessage(err))
		}
		if state != target.Running {
			missing = append(missing, dep)
			fmt.Fprintf(output.Out, "%s needs %s, which is %s.\n", e.t.Name(), dep.t.Name(), state)
		}
	}
	if len(missing) == 0 {
		return startTarget(cfg, e)
	}
	names := entryNames(missing)
	question := "Start " + names[0] + " first?"
	if len(names) > 1 {
		question = "Start " + strings.Join(names, ", then ") + ", first?"
	}
	if !confirm(question) {
		if len(names) == 1 {
			fmt.Fprintf(output.Out, "Not started. Start it first: dockguard start %s\n", names[0])
		} else {
			fmt.Fprintln(output.Out, "Not started. Start these first, in order:")
			for _, name := range names {
				fmt.Fprintf(output.Out, "  dockguard start %s\n", name)
			}
		}
		return 1, nil
	}
	for _, dep := range missing {
		state, err := dep.t.State()
		if state == target.Running {
			continue
		}
		if state == target.Unknown {
			return 1, fmt.Errorf("not starting %s: dependency %s state unknown: %s", e.t.Name(), dep.t.Name(), errMessage(err))
		}
		if code, err := startTarget(cfg, dep); code != 0 || err != nil {
			return code, err
		}
		state, err = waitForRunning(dep)
		if state != target.Running {
			if err != nil {
				return 1, fmt.Errorf("dependency %s not confirmed running (%s): %w", dep.t.Name(), state, err)
			}
			return 1, fmt.Errorf("dependency %s not confirmed running (%s)", dep.t.Name(), state)
		}
	}
	return startTarget(cfg, e)
}

func waitForRunning(e entry) (target.State, error) {
	deadline := now().Add(startConfirmTimeout)
	state, err := e.t.State()
	for state != target.Running && state != target.Unknown && now().Before(deadline) {
		sleep(startConfirmPoll)
		state, err = e.t.State()
	}
	return state, err
}

func runStop(cfg config.Config, entries []entry, e entry, eject bool) (int, error) {
	return stopWithDependencies(cfg, entries, e, eject, askDependency)
}

func stopWithDependencies(cfg config.Config, entries []entry, e entry, eject bool, confirm func(string) bool) (int, error) {
	hasDeps := false
	for _, other := range entries {
		hasDeps = hasDeps || len(other.cfg.DependsOn) > 0
	}
	if !hasDeps {
		return stopTarget(cfg, entries, e, eject)
	}
	g, err := newDependencyGraph(entries)
	if err != nil {
		return 1, err
	}
	active := map[string]bool{}
	var names []string
	for _, other := range entries {
		if g.needs(other.t.Name())[e.t.Name()] {
			state, _ := other.t.State()
			if state != target.Stopped {
				active[other.t.Name()] = true
				names = append(names, other.t.Name())
			}
		}
	}
	if len(names) == 0 {
		return stopTarget(cfg, entries, e, eject)
	}
	fmt.Fprintf(output.Out, "%s depend on %s and are running.\n", strings.Join(names, " and "), e.t.Name())
	if !confirm("Stop them first?") {
		fmt.Fprintf(output.Out, "Not stopped. Stop these first: %s\n", strings.Join(names, ", "))
		return 1, nil
	}
	for i := len(g.order) - 1; i >= 0; i-- {
		dep := g.order[i]
		if !active[dep.t.Name()] {
			continue
		}
		if code, err := stopTarget(cfg, entries, dep, false); code != 0 || err != nil {
			return code, err
		}
		state, err := dep.t.State()
		state, err = waitForStopped(dep, state, err)
		if state != target.Stopped {
			if err != nil {
				return 1, fmt.Errorf("dependent %s not confirmed stopped (%s): %w", dep.t.Name(), state, err)
			}
			return 1, fmt.Errorf("dependent %s not confirmed stopped (%s)", dep.t.Name(), state)
		}
	}
	return stopTarget(cfg, entries, e, eject)
}

func entryNames(entries []entry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.t.Name())
	}
	return names
}
