package app

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/holders"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/target/containersystem"
)

func holderCLI(t *testing.T, terminal bool, input string) {
	t.Helper()
	oldTerminal, oldInput := dependencyTerminal, dependencyInput
	oldService := holderService
	t.Cleanup(func() {
		dependencyTerminal, dependencyInput = oldTerminal, oldInput
		holderService = oldService
	})
	dependencyTerminal = func() bool { return terminal }
	dependencyInput = strings.NewReader(input)
	holderService = func() holders.Service {
		return holders.Service{
			PID: 99, UID: 501, Scan: holdersOf, Now: now, Sleep: sleep,
			Ancestors: func(int) ([]int, error) { return []int{98}, nil },
			Inspect: func(pid int) (platform.Process, error) {
				p := platform.Process{Name: "editor", Path: "/tmp/editor", UID: 501, Started: time.Unix(10, 1)}
				if pid == 30 {
					p.Name, p.Path = "mds", "/System/Library/Spotlight/mds"
				}
				return p, nil
			},
			Signal: func(int, syscall.Signal) error { t.Fatal("unexpected signal"); return nil },
		}
	}
}

var editor = platform.Holder{PID: 42, UID: 501, User: "501", Name: "editor"}

func TestHoldersCLI(t *testing.T) {
	for _, tt := range []struct {
		name, input        string
		stop, terminal     bool
		initial            []platform.Holder
		scanErr, rescanErr error
		code               int
		contains           []string
	}{
		{name: "AS123", code: 0, contains: []string{"no processes are using the drive"}},
		{name: "AS124", code: 1, initial: []platform.Holder{editor, {PID: 9, UID: 502, User: "502", Name: "other"}, {PID: 98, UID: 501, Name: "zsh"}, {PID: 30, UID: 501, Name: "mds"}}, contains: []string{"editor (PID 42) [stoppable]", "other (PID 9) [other user]", "owned by 502", "zsh (PID 98) [self]", "relaunch DockGuard", "mds (PID 30) [protected]", "Spotlight"}},
		{name: "AS125", code: 2, scanErr: errors.New("lsof timed out")},
		{name: "AS126", code: 1, stop: true, terminal: true, input: "no\n", initial: []platform.Holder{editor}, contains: []string{"Quit it? [Y/n]", "still using the drive"}},
		{name: "AS136", code: 1, stop: true, initial: []platform.Holder{editor}, contains: []string{"needs a terminal", "kill 42"}},
		{name: "AS136-clear", code: 1, stop: true, contains: []string{"needs a terminal"}},
		{name: "AS156", code: 2, stop: true, terminal: true, input: "n\n", initial: []platform.Holder{editor}, rescanErr: errors.New("lsof timed out")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t, true, false)
			holderCLI(t, tt.terminal, tt.input)
			scans := 0
			holdersOf = func(string) ([]platform.Holder, error) {
				scans++
				if scans == 1 {
					return tt.initial, tt.scanErr
				}
				return tt.initial, tt.rescanErr
			}
			args := []string{"holders"}
			if tt.stop {
				args = append(args, "--stop")
			}
			code, err := Run(args)
			if code != tt.code || (err != nil) != (code == 2 || strings.HasPrefix(tt.name, "AS136")) {
				t.Fatalf("Run = %d, %v; %s", code, err, h.out)
			}
			if code == 2 && !strings.Contains(err.Error(), "lsof timed out") {
				t.Errorf("error = %v", err)
			}
			got := h.out.String()
			for _, want := range tt.contains {
				if !strings.Contains(fmt.Sprint(got, err), want) {
					t.Errorf("missing %q in %s %v", want, got, err)
				}
			}
			if (!tt.stop || !tt.terminal) && strings.Contains(got, "Quit it?") {
				t.Errorf("unexpected prompt in %s", got)
			}
		})
	}
}

func TestHolderEjectCLI(t *testing.T) {
	for _, tt := range []struct {
		name, input              string
		args                     []string
		terminal, holding, eject bool
	}{
		{"AS137-AS144", "\n", []string{"--all"}, true, true, true},
		{"AS138", "n\n", []string{"--all"}, true, true, false},
		{"AS139", "", []string{"--all"}, false, true, false},
		{"AS143", "", []string{"--all"}, true, false, true},
		{"single-target", "yes\n", []string{"docker"}, true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			docker := &fake{name: "docker", states: states(target.Stopped)}
			h := setup(t, true, false, docker)
			holderCLI(t, tt.terminal, tt.input)
			holding := tt.holding
			holdersOf = func(string) ([]platform.Holder, error) {
				if holding {
					return []platform.Holder{editor}, nil
				}
				return nil, nil
			}
			var signals []syscall.Signal
			makeService := holderService
			holderService = func() holders.Service {
				s := makeService()
				s.Signal = func(_ int, signal syscall.Signal) error {
					if signal == 0 {
						return syscall.ESRCH
					}
					signals = append(signals, signal)
					holding = false
					return nil
				}
				return s
			}
			code, err := Run(append([]string{"stop", "--eject"}, tt.args...))
			if (code == 0 && err == nil) != tt.eject || h.ejected != tt.eject {
				t.Fatalf("Run = %d, %v; eject = %v; %s", code, err, h.ejected, h.out)
			}
			var expected []syscall.Signal
			if tt.eject && tt.holding {
				expected = []syscall.Signal{syscall.SIGTERM}
			}
			if !reflect.DeepEqual(expected, signals) {
				t.Errorf("signals = %v", signals)
			}
			out := h.out.String()
			if !tt.terminal && !strings.Contains(out, "still using the drive: editor (PID 42)\nkill 42\n") {
				t.Errorf("non-terminal output = %s", out)
			}
			if tt.name == "AS138" && !strings.Contains(out, "still using the drive: editor (PID 42) [stoppable]") {
				t.Errorf("refusal output = %s", out)
			}
			if !tt.holding {
				cfg, _ := loadConfig("")
				want := fmt.Sprintf("docker is already stopped.\nExternal volume ejected: %s\n", cfg.Drive)
				if out != want {
					t.Errorf("clean output = %q, want %q", out, want)
				}
			}
		})
	}
}

func TestPromptDefaults(t *testing.T) {
	for _, tt := range []struct {
		input            string
		defaultYes, want bool
	}{
		{"\n", true, true}, {"\n", false, false}, {"y\n", false, true},
		{" Yes \n", true, true}, {"n\n", true, false}, {"maybe\n", true, false},
		{"", true, false}, {"yes", true, false},
	} {
		t.Run(fmt.Sprintf("%q/default=%v", tt.input, tt.defaultYes), func(t *testing.T) {
			setup(t, true, false)
			holderCLI(t, true, tt.input)
			if got := cliPrompt()("Continue?", tt.defaultYes); got != tt.want {
				t.Errorf("answer = %v", got)
			}
		})
	}
}

type fakeUnmanagedSystem struct {
	*fake
	names, requested []string
	stopFailure      error
	stops            int
}

func (f *fakeUnmanagedSystem) State() (target.State, error) {
	if f.stopped {
		return target.Stopped, nil
	}
	return target.Running, nil
}

func (f *fakeUnmanagedSystem) Stop() (bool, error) {
	f.stops++
	if len(f.names) > 0 {
		return false, &containersystem.UnmanagedError{Names: f.names}
	}
	return f.fake.Stop()
}

func (f *fakeUnmanagedSystem) StopUnmanaged(name string) error {
	f.requested = append(f.requested, name)
	if f.stopFailure != nil {
		return f.stopFailure
	}
	var names []string
	for _, other := range f.names {
		if other != name {
			names = append(names, other)
		}
	}
	f.names = names
	return nil
}

func TestUnmanagedContainerCLI(t *testing.T) {
	for _, tt := range []struct {
		name, input       string
		args, names, want []string
		terminal, success bool
		err               error
	}{
		{"AS140", "yes\n", []string{"--all"}, []string{"buildkit"}, []string{"buildkit"}, true, true, nil},
		{"AS141", "no\n", []string{"--all"}, []string{"buildkit"}, nil, true, false, nil},
		{"AS142", "y\n", []string{"--all"}, []string{"buildkit"}, []string{"buildkit"}, true, false, errors.New("stop failed")},
		{"AS143", "", []string{"--all"}, nil, nil, true, true, nil},
		{"AS144", "\n", []string{"--all"}, []string{"buildkit"}, []string{"buildkit"}, true, true, nil},
		{"AS157", "y\nyes\n", []string{"system"}, []string{"buildkit", "scratch"}, []string{"buildkit", "scratch"}, true, true, nil},
		{"non-terminal", "yes\n", []string{"--all"}, []string{"buildkit"}, nil, false, false, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t, true, false)
			holderCLI(t, tt.terminal, tt.input)
			tc := config.Target{Name: "system", Kind: config.KindContainerSystem}
			s := &fakeUnmanagedSystem{fake: &fake{name: tc.Name, kind: tc.Kind}, names: tt.names, stopFailure: tt.err}
			buildTargets = func(config.Config) ([]entry, error) { return []entry{{t: s, cfg: tc}}, nil }
			code, err := Run(append([]string{"stop"}, tt.args...))
			if (code == 0 && err == nil) != tt.success || s.stopped != tt.success || !reflect.DeepEqual(s.requested, tt.want) {
				t.Fatalf("Run = %d, %v; system stopped %v; containers %v; %s", code, err, s.stopped, s.requested, h.out)
			}
			out := h.out.String()
			if (tt.terminal && len(tt.names) > 0) != strings.Contains(out, "Stop it? [Y/n]") {
				t.Errorf("unexpected prompt state: %s", out)
			}
			for _, name := range tt.want {
				if tt.err == nil && !strings.Contains(out, "container start "+name) {
					t.Errorf("missing restart hint in %s", out)
				}
			}
			if tt.err != nil && !strings.Contains(out, tt.err.Error()) {
				t.Errorf("missing stop error in %s", out)
			}
			if tt.name == "AS143" && out != "system stop requested.\n" {
				t.Errorf("clean output = %q", out)
			}
		})
	}
}

type fakeLegacyUnmanaged struct{ *fakeUnmanagedSystem }

func (*fakeLegacyUnmanaged) StopStack() (bool, error)    { return true, nil }
func (f *fakeLegacyUnmanaged) StopSystem() (bool, error) { return f.Stop() }

func TestLegacySystemDoesNotPromptAS158(t *testing.T) {
	h := setup(t, true, false)
	holderCLI(t, true, "yes\n")
	tc := config.Target{Name: "myapp", Kind: config.KindContainer, Container: &config.Container{StopSystem: true}}
	s := &fakeLegacyUnmanaged{&fakeUnmanagedSystem{fake: &fake{name: tc.Name, kind: tc.Kind}, names: []string{"buildkit"}}}
	buildTargets = func(config.Config) ([]entry, error) { return []entry{{t: s, cfg: tc}}, nil }
	code, err := Run([]string{"stop", "--all"})
	out := h.out.String()
	if code != 1 || err == nil || s.stops != 1 || len(s.requested) != 0 ||
		strings.Contains(out, "Stop it?") || !strings.Contains(out, "unmanaged containers are running (buildkit)") {
		t.Fatalf("Run = %d, %v; stops = %d; requested = %v; %s", code, err, s.stops, s.requested, out)
	}
}

func TestHoldersStopCLIOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name, input, message string
		ignoreTerm, denied   bool
		code                 int
		signals              []syscall.Signal
	}{
		{"AS127", "y\n", ": quit", false, false, 0, []syscall.Signal{syscall.SIGTERM}},
		{"AS128", "y\nyes\n", ": force-quit", true, false, 0, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}},
		{"AS129", "y\nno\n", ": still using the drive", true, false, 1, []syscall.Signal{syscall.SIGTERM}},
		{"AS135", "y\n", "operation not permitted; still using the drive", false, true, 1, []syscall.Signal{syscall.SIGTERM}},
		{"AS145", "y\n\n", ": still using the drive", true, false, 1, []syscall.Signal{syscall.SIGTERM}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := setup(t, true, false)
			holderCLI(t, true, tt.input)
			holding := true
			holdersOf = func(string) ([]platform.Holder, error) {
				if holding {
					return []platform.Holder{editor}, nil
				}
				return nil, nil
			}
			var sent []syscall.Signal
			makeService := holderService
			holderService = func() holders.Service {
				s := makeService()
				s.Signal = func(pid int, sig syscall.Signal) error {
					if pid != editor.PID {
						t.Fatalf("unexpected PID %d", pid)
					}
					if sig == 0 {
						if !holding {
							return syscall.ESRCH
						}
						return nil
					}
					sent = append(sent, sig)
					if tt.denied {
						return syscall.EPERM
					}
					if sig == syscall.SIGKILL || !tt.ignoreTerm {
						holding = false
					}
					return nil
				}
				return s
			}
			code, err := Run([]string{"holders", "--stop"})
			if err != nil || code != tt.code || !reflect.DeepEqual(sent, tt.signals) {
				t.Fatalf("Run = %d, %v; signals %v; %s", code, err, sent, h.out)
			}
			out := h.out.String()
			if !strings.Contains(out, tt.message) ||
				strings.Contains(out, "Force-quit it? Unsaved work will be lost. [y/N]") != tt.ignoreTerm ||
				strings.Contains(out, "no processes are using the drive") != (code == 0) {
				t.Errorf("output = %s", out)
			}
		})
	}
}

func TestHoldersCLIReleasesOwnCWDAS133(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(fmt.Sprintf("release=%v", release), func(t *testing.T) {
			h := setup(t, true, false)
			holderCLI(t, true, "yes\n")
			cfg, _ := loadConfig("")
			oldGetwd, oldChdir := getwd, chdir
			t.Cleanup(func() { getwd, chdir = oldGetwd, oldChdir })
			getwd = func() (string, error) { return cfg.Drive + "/work", nil }
			attempted := false
			chdir = func(path string) error {
				attempted = true
				if path != "/" {
					t.Fatalf("chdir %q", path)
				}
				if !release {
					return errors.New("working directory could not be released")
				}
				return nil
			}
			holdersOf = func(string) ([]platform.Holder, error) {
				if !attempted {
					t.Fatal("holder scan ran before the release attempt")
				}
				list := []platform.Holder{{Name: "zsh", PID: 98, UID: 501}}
				if !release {
					list = append(list, platform.Holder{Name: "dockguard", PID: 99, UID: 501})
				}
				return list, nil
			}
			code, err := Run([]string{"holders", "--stop"})
			out := h.out.String()
			if code != 1 || err != nil || !strings.Contains(out, "zsh (PID 98) [self]") ||
				strings.Contains(out, "dockguard (PID 99) [self]") == release ||
				!strings.Contains(out, "relaunch DockGuard from outside the drive") ||
				strings.Contains(out, "Quit it?") {
				t.Fatalf("Run = %d, %v; %s", code, err, out)
			}
		})
	}
}
