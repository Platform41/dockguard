// Package app parses commands and runs them (docs/specs/multi-target.md §2).
package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	appkind "github.com/platform41/dockguard/internal/target/app"
	"github.com/platform41/dockguard/internal/target/dockerdesktop"
	"github.com/platform41/dockguard/internal/target/postgres"
)

// defaultTarget is used by check, start and stop when no target is named (D-006).
const defaultTarget = "docker"

// Swappable for tests.
var (
	loadConfig   = config.Load
	buildTargets = defaultBuildTargets
	driveMounted = checks.DriveMounted
	ejectVolume  = platform.EjectVolume
	sleep        = time.Sleep
	now          = time.Now
)

// Eject rule timings (spec §2.3).
var (
	stopConfirmTimeout = 30 * time.Second
	stopConfirmPoll    = 1 * time.Second
)

type options struct {
	Command    string
	Target     string
	ConfigPath string
	Eject      bool
}

// entry pairs a target with its config.
type entry struct {
	t   target.Target
	cfg config.Target
}

func Run(args []string) (int, error) {
	opts, err := parseArgs(args)
	if err != nil {
		return 1, err
	}

	switch opts.Command {
	case "":
		output.PrintUsage()
		return 1, nil
	case "help", "--help", "-h":
		output.PrintUsage()
		return 0, nil
	}

	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return 1, err
	}

	entries, err := buildTargets(cfg)
	if err != nil {
		return 1, err
	}

	switch opts.Command {
	case "list":
		return runList(cfg, entries)
	case "status":
		return runStatus(cfg, entries)
	}

	name := opts.Target
	if name == "" {
		name = defaultTarget
	}
	selected, ok := find(entries, name)
	if !ok {
		return 1, fmt.Errorf("unknown target %q; configured targets: %s", name, strings.Join(cfg.Names(), ", "))
	}

	switch opts.Command {
	case "check":
		return runCheck(cfg, selected)
	case "start":
		return runStart(cfg, selected)
	default:
		return runStop(cfg, entries, selected, opts.Eject)
	}
}

var commands = map[string]bool{"list": true, "status": true, "check": true, "start": true, "stop": true, "help": true, "--help": true, "-h": true}
var takesTarget = map[string]bool{"check": true, "start": true, "stop": true}

func parseArgs(args []string) (options, error) {
	if len(args) == 0 {
		return options{}, nil
	}

	opts := options{Command: args[0]}
	if !commands[opts.Command] {
		return options{}, fmt.Errorf("unknown command %q", opts.Command)
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--config":
			if i+1 >= len(args) {
				return options{}, errors.New("--config requires a path")
			}
			opts.ConfigPath = args[i+1]
			i++
		case arg == "--eject":
			opts.Eject = true
		case strings.HasPrefix(arg, "-"):
			return options{}, fmt.Errorf("unknown argument %q", arg)
		case takesTarget[opts.Command] && opts.Target == "":
			opts.Target = arg
		default:
			return options{}, fmt.Errorf("unexpected argument %q", arg)
		}
	}

	if opts.Eject && opts.Command != "stop" {
		return options{}, errors.New("--eject is only supported with the stop command")
	}

	return opts, nil
}

func defaultBuildTargets(cfg config.Config) ([]entry, error) {
	entries := make([]entry, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		switch t.Kind {
		case config.KindDockerDesktop:
			entries = append(entries, entry{t: dockerdesktop.New(t, cfg.Drive, cfg.Legacy), cfg: t})
		case config.KindApp:
			entries = append(entries, entry{t: appkind.New(t, cfg.Drive), cfg: t})
		case config.KindPostgres:
			entries = append(entries, entry{t: postgres.New(t, cfg.Drive), cfg: t})
		default:
			return nil, fmt.Errorf("target %q: kind %q is not supported", t.Name, t.Kind)
		}
	}
	return entries, nil
}

func find(entries []entry, name string) (entry, bool) {
	for _, e := range entries {
		if e.t.Name() == name {
			return e, true
		}
	}
	return entry{}, false
}

// display is how messages name a target; Docker keeps its v0.1 wording.
func display(e entry) string {
	switch {
	case e.t.Kind() == config.KindDockerDesktop:
		return "Docker Desktop"
	case e.cfg.App != nil:
		return e.cfg.App.Name
	}
	return e.t.Name()
}

// preflight returns the drive check, the target's checks and, if asked, the already-running
// guard. When the drive isn't mounted, the target's checks are skipped (spec §5).
func preflight(cfg config.Config, e entry, withGuard bool) []checks.Item {
	items := checks.Drive(cfg.Drive)
	if !driveMounted(cfg.Drive) {
		return append(items, checks.SkipAll(e.t.Checks())...)
	}

	items = append(items, e.t.Checks()...)
	if withGuard && e.cfg.FailIfAlreadyRunning {
		items = append(items, guard(e))
	}
	return items
}

// guard is C8.
func guard(e entry) checks.Item {
	name := e.t.Name() + " not already running"
	if e.t.Kind() == config.KindDockerDesktop {
		name = "docker desktop not already running"
	}

	state, err := e.t.State()
	switch state {
	case target.Stopped:
		return checks.Item{Name: name, OK: true, Message: "not running"}
	case target.Running:
		return checks.Item{Name: name, OK: false, Message: display(e) + " is already running"}
	case target.Partial:
		return checks.Item{Name: name, OK: false, Message: display(e) + " is partially running"}
	default:
		return checks.Item{Name: name, OK: false, Message: errMessage(err)}
	}
}

func runList(cfg config.Config, entries []entry) (int, error) {
	mounted := driveMounted(cfg.Drive)
	output.PrintHeader(cfg.Path, cfg.Drive, mounted)

	rows := make([]output.Row, 0, len(entries))
	for _, e := range entries {
		state := "drive not mounted"
		if mounted {
			s, _ := e.t.State()
			state = s.String()
		}
		rows = append(rows, output.Row{Name: e.t.Name(), Kind: e.t.Kind(), State: state, Path: e.t.MainPath()})
	}
	output.PrintList(rows)
	return 0, nil
}

func runStatus(cfg config.Config, entries []entry) (int, error) {
	mounted := driveMounted(cfg.Drive)
	output.PrintHeader(cfg.Path, cfg.Drive, mounted)
	if cfg.Legacy {
		output.PrintLegacyNotice()
	}

	ready := true
	for _, e := range entries {
		state := "drive not mounted"
		if mounted {
			s, err := e.t.State()
			state = s.String()
			if s == target.Unknown {
				// Safety can't be established without a state, so the target isn't ready.
				state += " (" + errMessage(err) + ")"
				ready = false
			}
		}
		output.PrintTargetHeading(e.t.Name(), e.t.Kind(), state)

		items := preflight(cfg, e, false)
		output.PrintItems(items)
		if !checks.AllOK(items) {
			ready = false
		}
	}

	output.PrintSummary(ready)
	if ready {
		return 0, nil
	}
	return 1, nil
}

func runCheck(cfg config.Config, e entry) (int, error) {
	items := preflight(cfg, e, true)
	output.PrintItems(items)
	if checks.AllOK(items) {
		return 0, nil
	}
	return 1, errors.New("preflight checks failed")
}

func runStart(cfg config.Config, e entry) (int, error) {
	items := preflight(cfg, e, true)
	output.PrintItems(items)
	if !checks.AllOK(items) {
		return 1, fmt.Errorf("refusing to start %s because preflight checks failed", display(e))
	}

	if err := e.t.Start(); err != nil {
		return 1, err
	}

	output.PrintStarted(display(e))
	return 0, nil
}

func runStop(cfg config.Config, entries []entry, e entry, eject bool) (int, error) {
	stopRequested, err := e.t.Stop()
	if err != nil {
		return 1, err
	}
	if stopRequested {
		output.PrintStopped(display(e))
	} else {
		output.PrintAlreadyStopped(display(e))
	}

	if !eject {
		return 0, nil
	}

	justStopped := map[string]bool{}
	if stopRequested {
		justStopped[e.t.Name()] = true
	}
	return ejectIfAllStopped(cfg, entries, justStopped)
}

// ejectIfAllStopped is the eject rule (spec §2.3): probe every target again, waiting for the ones
// just stopped, and eject only when every target is confirmed stopped.
func ejectIfAllStopped(cfg config.Config, entries []entry, justStopped map[string]bool) (int, error) {
	if !driveMounted(cfg.Drive) {
		return 1, fmt.Errorf("drive %s is not mounted; nothing to eject", cfg.Drive)
	}

	var blockers []output.Blocker
	for _, e := range entries {
		state, err := e.t.State()
		if justStopped[e.t.Name()] {
			state, err = waitForStopped(e, state, err)
		}
		if state != target.Stopped {
			reason := ""
			if state == target.Unknown {
				reason = errMessage(err)
			}
			blockers = append(blockers, output.Blocker{Name: e.t.Name(), State: state.String(), Reason: reason})
		}
	}

	if len(blockers) > 0 {
		output.PrintEjectRefused(cfg.Drive, blockers)
		return 1, errors.New("drive not ejected")
	}

	if err := ejectVolume(cfg.Drive); err != nil {
		return 1, err
	}
	output.PrintEjected(cfg.Drive)
	return 0, nil
}

func waitForStopped(e entry, state target.State, err error) (target.State, error) {
	deadline := now().Add(stopConfirmTimeout)
	for state != target.Stopped && now().Before(deadline) {
		sleep(stopConfirmPoll)
		state, err = e.t.State()
	}
	return state, err
}

func errMessage(err error) string {
	if err == nil {
		return "state could not be determined"
	}
	return err.Error()
}
