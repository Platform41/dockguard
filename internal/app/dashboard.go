package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/term"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/drive"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/ui"
)

// Swappable for tests.
var (
	isTerminal = stdioIsTerminal
	runUI      = ui.Run
	getwd      = os.Getwd
	chdir      = os.Chdir
)

// runDashboard opens `dockguard ui` (spec §2.4). Outside a terminal it refuses (AS-35).
func runDashboard(cfg config.Config, entries []entry) (int, error) {
	if !isTerminal() {
		return 1, errors.New("dockguard ui needs a terminal; use `dockguard list` or `dockguard status` instead")
	}
	if err := releaseDriveCWD(cfg.Drive); err != nil {
		return 1, err
	}
	if err := runUI(&controller{cfg: cfg, entries: entries}); err != nil {
		return 1, err
	}
	return 0, nil
}

// releaseDriveCWD prevents DockGuard itself from holding the drive open. Config is fully loaded
// before this runs; configured paths are absolute.
func releaseDriveCWD(drive string) error {
	cwd, err := getwd()
	if err != nil {
		return fmt.Errorf("inspect DockGuard working directory: %w", err)
	}
	rel, err := filepath.Rel(filepath.Clean(drive), filepath.Clean(cwd))
	if err != nil {
		return fmt.Errorf("compare DockGuard working directory with %s: %w", drive, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if err := chdir("/"); err != nil {
		return fmt.Errorf("release DockGuard working directory on %s: %w; relaunch DockGuard from outside the drive", drive, err)
	}
	return nil
}

func stdioIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// controller implements ui.Controller with the same functions the CLI commands use, so the
// dashboard follows exactly the same rules.
type controller struct {
	cfg     config.Config
	entries []entry
	// mu serialises actions: they temporarily redirect output.Out.
	mu sync.Mutex
}

func (c *controller) Snapshot() ui.Snapshot {
	ds := driveStatus(c.cfg)
	s := ui.Snapshot{ConfigPath: c.cfg.Path, Drive: c.cfg.Drive, Mounted: ds.Mounted()}
	if c.cfg.DriveUUID != "" {
		s.DriveStatus = string(ds.State)
		s.DriveMessage = ds.Message
		s.CanMount = ds.State == drive.NotMounted
	}
	if s.Mounted {
		s.UsedPct, s.FreeGB, s.TotalGB = diskUsage(c.cfg.Drive)
	}
	// Probe every target at once, so one slow probe doesn't hold up the others.
	s.Rows = make([]ui.Row, len(c.entries))
	var wg sync.WaitGroup
	if s.Mounted {
		wg.Add(1)
		go func() {
			defer wg.Done()
			holders, err := holdersOf(c.cfg.Drive)
			for _, h := range holders {
				s.Holders = append(s.Holders, ui.Holder{Name: h.Name, PID: h.PID})
			}
			if err != nil {
				s.HolderErr = err.Error()
			}
		}()
	}
	for i, e := range c.entries {
		s.Rows[i] = ui.Row{Name: e.t.Name(), Kind: e.t.Kind(), Path: e.t.MainPath()}
		s.Rows[i].DependsOn = append([]string{}, e.cfg.DependsOn...)
		if e.cfg.Container != nil {
			s.Rows[i].Ephemeral = e.cfg.Container.Ephemeral
		}
		if !s.Mounted {
			continue
		}
		wg.Add(1)
		go func(i int, e entry) {
			defer wg.Done()
			state, err := e.t.State()
			s.Rows[i].State = state
			if err != nil {
				s.Rows[i].Err = err.Error()
			}
		}(i, e)
	}
	wg.Wait()
	return s
}

func diskUsage(path string) (usedPct float64, freeGB, totalGB uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil || st.Blocks == 0 {
		return 0, 0, 0
	}
	const gb = 1024 * 1024 * 1024
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used := total - st.Bfree*uint64(st.Bsize)
	return float64(used) / float64(total) * 100, free / gb, total / gb
}

func (c *controller) Run(action ui.Action, name string) (bool, string) {
	return c.run(action, name, nil)
}

func (c *controller) Plan(action ui.Action, name string) (ui.Confirmation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := find(c.entries, name)
	if !ok {
		return ui.Confirmation{}, fmt.Errorf("unknown target %q", name)
	}
	var p dependencyPlan
	var err error
	switch action {
	case ui.Start:
		p, err = planStart(c.entries, e)
	case ui.Stop:
		p, err = planStop(c.entries, e)
	}
	return ui.Confirmation{Action: action, Target: name, Question: p.question, Detail: p.detail, Targets: append([]string{}, p.names...)}, err
}

func (c *controller) RunConfirmed(action ui.Action, name string, approval ui.Confirmation) (bool, string) {
	return c.run(action, name, &approval)
}

func (c *controller) run(action ui.Action, name string, approval *ui.Confirmation) (bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var buf bytes.Buffer
	previous := output.Out
	output.Out = &buf
	defer func() { output.Out = previous }()
	confirm := func(p dependencyPlan) bool {
		return approval != nil && approval.Action == action && approval.Target == name && approval.Question == p.question && slices.Equal(approval.Targets, p.names)
	}

	var code int
	var err error
	if action == ui.Mount {
		code, err = runMount(c.cfg)
	} else if action == ui.StopAllEject {
		code, err = runStopAll(c.cfg, c.entries, true, nil)
	} else {
		e, ok := find(c.entries, name)
		if !ok {
			return false, fmt.Sprintf("unknown target %q", name)
		}
		switch action {
		case ui.Start:
			code, err = startWithDependencies(c.cfg, c.entries, e, confirm)
		case ui.Stop:
			code, err = stopWithDependencies(c.cfg, c.entries, e, false, confirm, nil)
		case ui.Check:
			code, err = runCheck(c.cfg, e)
		}
	}
	if err != nil {
		fmt.Fprintln(&buf, err)
	}
	return code == 0 && err == nil, buf.String()
}
