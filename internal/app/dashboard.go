package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/ui"
)

// Swappable for tests.
var (
	isTerminal = stdioIsTerminal
	runUI      = ui.Run
)

// runDashboard opens `dockguard ui` (spec §2.4). Outside a terminal it refuses (AS-35).
func runDashboard(cfg config.Config, entries []entry) (int, error) {
	if !isTerminal() {
		return 1, errors.New("dockguard ui needs a terminal; use `dockguard list` or `dockguard status` instead")
	}
	if err := runUI(&controller{cfg: cfg, entries: entries}); err != nil {
		return 1, err
	}
	return 0, nil
}

func stdioIsTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
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
	s := ui.Snapshot{ConfigPath: c.cfg.Path, Drive: c.cfg.Drive, Mounted: driveMounted(c.cfg.Drive)}
	if s.Mounted {
		s.UsedPct, s.FreeGB, s.TotalGB = diskUsage(c.cfg.Drive)
	}
	// Probe every target at once, so one slow probe doesn't hold up the others.
	s.Rows = make([]ui.Row, len(c.entries))
	var wg sync.WaitGroup
	for i, e := range c.entries {
		s.Rows[i] = ui.Row{Name: e.t.Name(), Kind: e.t.Kind(), Path: e.t.MainPath()}
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
	c.mu.Lock()
	defer c.mu.Unlock()

	var buf bytes.Buffer
	previous := output.Out
	output.Out = &buf
	defer func() { output.Out = previous }()

	var code int
	var err error
	if action == ui.StopAllEject {
		code, err = runStopAll(c.cfg, c.entries, true)
	} else {
		e, ok := find(c.entries, name)
		if !ok {
			return false, fmt.Sprintf("unknown target %q", name)
		}
		switch action {
		case ui.Start:
			code, err = runStart(c.cfg, e)
		case ui.Stop:
			code, err = runStop(c.cfg, c.entries, e, false)
		case ui.Check:
			code, err = runCheck(c.cfg, e)
		}
	}
	if err != nil {
		fmt.Fprintln(&buf, err)
	}
	return code == 0 && err == nil, buf.String()
}
