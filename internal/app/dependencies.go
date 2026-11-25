package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/holders"
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
	order      []entry
	configured []entry
	byName     map[string]entry
}

func newDependencyGraph(entries []entry) (dependencyGraph, error) {
	g := dependencyGraph{byName: map[string]entry{}, configured: append([]entry{}, entries...)}
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

// cliPrompt keeps a single reader across a command's questions so that
// answers buffered while reading one line remain available to the next question.
func cliPrompt() holders.Prompt {
	if !dependencyTerminal() {
		return nil
	}
	input := bufio.NewReader(dependencyInput)
	return func(question string, defaultYes bool) bool {
		suffix := " [y/N] "
		if defaultYes {
			suffix = " [Y/n] "
		}
		fmt.Fprint(output.Out, question+suffix)
		answer, err := input.ReadString('\n')
		fmt.Fprintln(output.Out)
		if err != nil {
			// Incomplete input at EOF is not consent.
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes" || (defaultYes && answer == "")
	}
}

// askDependency is CLI-only. Dashboard actions must not compete with the TUI for stdin.
func askDependency(question string) bool {
	ask := cliPrompt()
	return ask != nil && ask(question, false)
}

// dependencyPlan is read-only until its caller explicitly confirms it.
type dependencyPlan struct {
	items            []entry
	names            []string
	question, detail string
}

func planStart(entries []entry, e entry) (dependencyPlan, error) {
	p := dependencyPlan{}
	if len(e.cfg.DependsOn) == 0 {
		return p, nil
	}
	g, err := newDependencyGraph(entries)
	if err != nil {
		return p, err
	}
	needed := g.needs(e.t.Name())
	var notices []string
	for _, dep := range g.order {
		if !needed[dep.t.Name()] {
			continue
		}
		state, err := dep.t.State()
		if state == target.Unknown {
			return p, fmt.Errorf("not starting %s: dependency %s state unknown: %s", e.t.Name(), dep.t.Name(), errMessage(err))
		}
		if state != target.Running {
			p.items = append(p.items, dep)
			notices = append(notices, fmt.Sprintf("%s needs %s, which is %s.", e.t.Name(), dep.t.Name(), state))
		}
	}
	p.names = entryNames(p.items)
	p.detail = strings.Join(notices, "\n")
	if len(p.names) == 1 {
		p.question = "Start " + p.names[0] + " first?"
	}
	if len(p.names) > 1 {
		p.question = "Start " + strings.Join(p.names, ", then ") + ", first?"
	}
	return p, nil
}

func runStart(cfg config.Config, entries []entry, e entry) (int, error) {
	return startWithDependencies(cfg, entries, e, confirmDependency)
}

func confirmDependency(p dependencyPlan) bool { return askDependency(p.question) }

func startWithDependencies(cfg config.Config, entries []entry, e entry, confirm func(dependencyPlan) bool) (int, error) {
	p, err := planStart(entries, e)
	if err != nil {
		return 1, err
	}
	if len(p.items) == 0 {
		return startTarget(cfg, e)
	}
	fmt.Fprintln(output.Out, p.detail)
	if !confirm(p) {
		if len(p.names) == 1 {
			fmt.Fprintf(output.Out, "Not started. Start it first: dockguard start %s\n", p.names[0])
		} else {
			fmt.Fprintln(output.Out, "Not started. Start these first, in order:")
			for _, name := range p.names {
				fmt.Fprintf(output.Out, "  dockguard start %s\n", name)
			}
		}
		return 1, nil
	}
	for _, dep := range p.items {
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

func planStop(entries []entry, e entry) (dependencyPlan, error) {
	p := dependencyPlan{}
	hasDeps := false
	for _, other := range entries {
		hasDeps = hasDeps || len(other.cfg.DependsOn) > 0
	}
	if !hasDeps {
		return p, nil
	}
	g, err := newDependencyGraph(entries)
	if err != nil {
		return p, err
	}
	active := map[string]bool{}
	for _, other := range entries {
		if g.needs(other.t.Name())[e.t.Name()] {
			state, _ := other.t.State()
			if state != target.Stopped {
				active[other.t.Name()] = true
				p.names = append(p.names, other.t.Name())
			}
		}
	}
	if len(p.names) == 0 {
		return p, nil
	}
	for _, dep := range g.stopOrder() {
		if active[dep.t.Name()] {
			p.items = append(p.items, dep)
		}
	}
	p.question = "Stop them first?"
	p.detail = dependentNotice(p.names, e.t.Name())
	return p, nil
}

func runStop(cfg config.Config, entries []entry, e entry, eject bool) (int, error) {
	prompts := &stopPrompts{}
	return stopWithDependencies(cfg, entries, e, eject, func(p dependencyPlan) bool {
		ask := prompts.question()
		return ask != nil && ask(p.question, false)
	}, prompts)
}

func stopWithDependencies(cfg config.Config, entries []entry, e entry, eject bool, confirm func(dependencyPlan) bool, prompts *stopPrompts) (int, error) {
	p, err := planStop(entries, e)
	if err != nil {
		return 1, err
	}
	if len(p.items) == 0 {
		return stopTarget(cfg, entries, e, eject, prompts)
	}
	fmt.Fprintln(output.Out, p.detail)
	if !confirm(p) {
		fmt.Fprintf(output.Out, "Not stopped. Stop these first: %s\n", strings.Join(p.names, ", "))
		return 1, nil
	}
	for _, dep := range p.items {
		if code, err := stopTarget(cfg, entries, dep, false, prompts); code != 0 || err != nil {
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
	return stopTarget(cfg, entries, e, eject, prompts)
}

func entryNames(entries []entry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.t.Name())
	}
	return names
}

func dependentNotice(names []string, name string) string {
	if len(names) == 1 {
		return fmt.Sprintf("%s depends on %s and is running.", names[0], name)
	}
	return fmt.Sprintf("%s depend on %s and are running.", strings.Join(names, " and "), name)
}

func printStopSystemDeprecation(e entry) {
	if e.cfg.Container != nil && e.cfg.Container.StopSystem {
		fmt.Fprintf(output.Out, "%s: stop_system is deprecated; add a container-system target and depends_on (see README).\n", e.t.Name())
	}
}
