package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/holders"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target/containersystem"
)

var holderService = func() holders.Service {
	return holders.Service{
		PID: os.Getpid(), UID: os.Geteuid(),
		Scan: holdersOf, Inspect: platform.InspectProcess, Signal: platform.Signal,
		Ancestors: platform.Ancestors, Now: now, Sleep: sleep,
	}
}

func runHolders(cfg config.Config, stop bool) (int, error) {
	ds := driveStatus(cfg)
	if !ds.Mounted() {
		return 2, fmt.Errorf("couldn't inspect processes using the drive: %s: %s", cfg.Drive, ds.Message)
	}

	cwdErr := releaseDriveCWD(cfg.Drive)
	s := holderService()
	list, err := s.Scan(cfg.Drive)
	candidates := s.Classify(list)
	printHolders(candidates)
	if err != nil {
		return 2, holderInspectionError(err)
	}
	if cwdErr != nil {
		fmt.Fprintln(output.Out, cwdErr)
	}
	if !stop || len(candidates) == 0 {
		return holderExit(candidates), nil
	}

	ask := cliPrompt()
	if ask == nil {
		printManualQuit(candidates)
		return 1, errors.New("dockguard holders --stop needs a terminal")
	}
	remaining, err := stopHolders(cfg.Drive, s, candidates, ask)
	printHolders(remaining)
	if err != nil {
		return 2, holderInspectionError(err)
	}
	return holderExit(remaining), nil
}

func holderExit(candidates []holders.Candidate) int {
	if len(candidates) == 0 {
		fmt.Fprintln(output.Out, "no processes are using the drive")
		return 0
	}
	return 1
}

func printHolders(candidates []holders.Candidate) {
	for _, c := range candidates {
		fmt.Fprintln(output.Out, c)
	}
}

func printManualQuit(candidates []holders.Candidate) {
	for _, c := range candidates {
		if c.CanStop() {
			fmt.Fprintf(output.Out, "kill %d\n", c.PID)
		}
	}
}

func holderInspectionError(err error) error {
	return fmt.Errorf("couldn't inspect processes using the drive: %w", err)
}

func stopHolders(mount string, s holders.Service, candidates []holders.Candidate, ask holders.Prompt) ([]holders.Candidate, error) {
	for _, c := range candidates {
		result, scanErr := s.Stop(mount, c, ask)
		if scanErr != nil {
			return nil, scanErr
		}
		if result.Err != nil {
			fmt.Fprintf(output.Out, "%s: %v; still using the drive\n", c.Holder, result.Err)
		} else if result.Message != "" {
			fmt.Fprintf(output.Out, "%s: %s\n", c.Holder, result.Message)
		}
	}
	list, err := s.Scan(mount)
	return s.Classify(list), err
}

// A CLI stop creates its prompt lazily and shares it across dependency, container
// and holder questions. The dashboard passes nil and never reads CLI input.
type stopPrompts struct {
	checked bool
	ask     holders.Prompt
}

func (p *stopPrompts) question() holders.Prompt {
	if p == nil {
		return nil
	}
	if !p.checked {
		p.ask = cliPrompt()
		p.checked = true
	}
	return p.ask
}

func ejectHolderPreflight(mount string, prompts *stopPrompts) error {
	cwdErr := releaseDriveCWD(mount)
	if cwdErr != nil && prompts == nil {
		output.PrintNotEjecting(mount, cwdErr.Error())
		return errors.New("drive not ejected")
	}

	s := holderService()
	list, err := s.Scan(mount)
	if cwdErr != nil {
		printHolders(s.Classify(list))
		output.PrintNotEjecting(mount, cwdErr.Error())
		return errors.New("drive not ejected")
	}
	if err != nil || len(list) == 0 {
		return reportHolderPreflight(mount, platform.DescribeHolders(list), err)
	}

	ask := prompts.question()
	if ask == nil {
		err := reportHolderPreflight(mount, platform.DescribeHolders(list), nil)
		if prompts != nil {
			printManualQuit(s.Classify(list))
		}
		return err
	}

	candidates := s.Classify(list)
	printHolders(candidates)
	remaining, err := stopHolders(mount, s, candidates, ask)
	labels := make([]string, 0, len(remaining))
	for _, c := range remaining {
		labels = append(labels, c.String())
	}
	return reportHolderPreflight(mount, strings.Join(labels, ", "), err)
}

func reportHolderPreflight(mount, names string, err error) error {
	switch {
	case names != "":
		reason := "still using the drive: " + names
		if err != nil {
			reason += "; process inspection incomplete: " + err.Error()
		}
		output.PrintNotEjecting(mount, reason)
	case err != nil:
		output.PrintNotEjecting(mount, holderInspectionError(err).Error())
	default:
		return nil
	}
	return errors.New("drive not ejected")
}

type unmanagedContainerStopper interface {
	StopUnmanaged(string) error
}

func stopWithUnmanagedContainers(e entry, prompts *stopPrompts) (bool, error) {
	requested, err := e.t.Stop()
	var blocked *containersystem.UnmanagedError
	system, ok := e.t.(unmanagedContainerStopper)
	if e.cfg.Kind != config.KindContainerSystem || !ok || !errors.As(err, &blocked) {
		return requested, err
	}

	ask := prompts.question()
	if ask == nil {
		return requested, err
	}
	var failures []error
	for _, name := range blocked.Names {
		question := fmt.Sprintf("Container %s is running and isn't managed by DockGuard. Stop it?", name)
		if !ask(question, true) {
			continue
		}
		if err := system.StopUnmanaged(name); err != nil {
			failures = append(failures, err)
			continue
		}
		fmt.Fprintf(output.Out, "Container %s stopped. Restart it with: container start %s\n", name, name)
	}
	if len(failures) > 0 {
		return false, errors.Join(failures...)
	}
	// Probe again even for declined answers: a container may have exited while
	// the user was deciding, or a new unmanaged container may have started.
	return e.t.Stop()
}
