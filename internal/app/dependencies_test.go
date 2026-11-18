package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
)

type dependencyFake struct {
	*fake
	deps                []string
	state               target.State
	startErr            error
	holdStart, holdStop bool
	calls               *[]string
	checked             int
}

func (f *dependencyFake) State() (target.State, error) { f.probes++; return f.state, f.stateErr }
func (f *dependencyFake) Checks() []checks.Item        { f.checked++; return f.fake.Checks() }
func (f *dependencyFake) Start() error {
	*f.calls = append(*f.calls, "start "+f.name)
	f.started = true
	if f.startErr == nil && !f.holdStart {
		f.state = target.Running
	}
	return f.startErr
}
func (f *dependencyFake) Stop() (bool, error) {
	*f.calls = append(*f.calls, "stop "+f.name)
	if f.stopErr != nil {
		return false, f.stopErr
	}
	f.stopped = true
	if !f.holdStop {
		f.state = target.Stopped
	}
	return true, nil
}

func dependencySetup(t *testing.T, terminal bool, answer string, nodes ...*dependencyFake) (*harness, *[]string, config.Config, []entry) {
	t.Helper()
	var bases []*fake
	calls := []string{}
	for _, n := range nodes {
		if n.fake == nil {
			t.Fatal("fake required")
		}
		n.calls = &calls
		n.checksOK = true
		bases = append(bases, n.fake)
	}
	h := setup(t, true, false, bases...)
	cfg, _ := loadConfig("")
	var entries []entry
	for i, n := range nodes {
		cfg.Targets[i].DependsOn = n.deps
		entries = append(entries, entry{t: n, cfg: cfg.Targets[i]})
	}
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	oldTTY, oldInput := dependencyTerminal, dependencyInput
	t.Cleanup(func() { dependencyTerminal, dependencyInput = oldTTY, oldInput })
	dependencyTerminal = func() bool { return terminal }
	dependencyInput = strings.NewReader(answer)
	return h, &calls, cfg, entries
}
func node(name string, state target.State, deps ...string) *dependencyFake {
	return &dependencyFake{fake: &fake{name: name}, state: state, deps: deps}
}

// AS-107 through AS-110: ask only for a missing prerequisite; only complete y/yes answers consent.
func TestStartDependencyAnswers(t *testing.T) {
	for _, tt := range []struct {
		name, answer   string
		terminal, want bool
	}{
		{"y", "y\n", true, true}, {"yes", "YeS\n", true, true}, {"upper", "Y\n", true, true},
		{"enter", "\n", true, false}, {"no", "n\n", true, false}, {"other", "sure\n", true, false},
		{"EOF", "", true, false}, {"partial EOF", "y", true, false}, {"not terminal", "y\n", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base, app := node("container", target.Stopped), node("myapp", target.Stopped, "container")
			h, calls, _, _ := dependencySetup(t, tt.terminal, tt.answer, base, app)
			code, err := Run([]string{"start", "myapp"})
			if tt.want {
				if code != 0 || err != nil || !reflect.DeepEqual(*calls, []string{"start container", "start myapp"}) || base.checked != 1 || app.checked != 1 {
					t.Fatalf("%d %v %v checks %d/%d", code, err, *calls, base.checked, app.checked)
				}
			} else if code != 1 || len(*calls) != 0 || app.checked != 0 || !strings.Contains(h.out.String(), "Not started. Start it first: dockguard start container\n") {
				t.Fatalf("%d %v %v output %s", code, err, *calls, h.out)
			}
			if strings.Contains(h.out.String(), "[y/N]") != tt.terminal {
				t.Fatalf("prompt: %s", h.out)
			}
		})
	}
}

func TestStartSkipsRunningDependencies(t *testing.T) {
	base, app := node("container", target.Running), node("myapp", target.Stopped, "container")
	h, calls, _, _ := dependencySetup(t, true, "", base, app)
	code, err := Run([]string{"start", "myapp"})
	if code != 0 || err != nil || strings.Contains(h.out.String(), "[y/N]") || !reflect.DeepEqual(*calls, []string{"start myapp"}) || base.checked != 0 {
		t.Fatalf("%d %v %v %s", code, err, *calls, h.out)
	}
}

// AS-110/AS-111: one chain prompt, deepest first, and ordered copy-paste refusal commands.
func TestStartDependencyChain(t *testing.T) {
	for _, terminal := range []bool{true, false} {
		for _, answer := range []string{"y\n", "n\n"} {
			t.Run(answer+string(rune('0'+boolInt(terminal))), func(t *testing.T) {
				app, base, system := node("myapp", target.Stopped, "container"), node("docker", target.Stopped), node("container", target.Stopped, "docker")
				h, calls, _, _ := dependencySetup(t, terminal, answer, app, base, system)
				code, err := Run([]string{"start", "myapp"})
				if terminal && answer == "y\n" {
					if code != 0 || err != nil || !reflect.DeepEqual(*calls, []string{"start docker", "start container", "start myapp"}) {
						t.Fatalf("%d %v %v", code, err, *calls)
					}
					if strings.Count(h.out.String(), "[y/N]") != 1 || !strings.Contains(h.out.String(), "Start docker, then container, first? [y/N]") {
						t.Fatal(h.out)
					}
				} else if code != 1 || len(*calls) != 0 || !strings.Contains(h.out.String(), "Not started. Start these first, in order:\n  dockguard start docker\n  dockguard start container\n") {
					t.Fatalf("%d %v %v %s", code, err, *calls, h.out)
				}
			})
		}
	}
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestChainRefusalOmitsRunningDependencies(t *testing.T) {
	h, _, _, _ := dependencySetup(t, false, "", node("myapp", target.Stopped, "container"), node("docker", target.Running), node("container", target.Partial, "docker"))
	if code, _ := Run([]string{"start", "myapp"}); code != 1 || !strings.Contains(h.out.String(), "Not started. Start it first: dockguard start container") || strings.Contains(h.out.String(), "dockguard start docker") {
		t.Fatal(h.out)
	}
}

// AS-112/113: fail before target preflight and do not continue after failure.
func TestStartDependencyFailures(t *testing.T) {
	for _, failure := range []string{"unknown", "preflight", "start", "not running"} {
		t.Run(failure, func(t *testing.T) {
			base, system, app := node("base", target.Stopped), node("system", target.Stopped, "base"), node("myapp", target.Stopped, "system")
			h, _, _, _ := dependencySetup(t, true, "y\n", base, system, app)
			switch failure {
			case "unknown":
				system.state = target.Unknown
				system.stateErr = errors.New("probe denied")
			case "preflight":
				base.checksOK = false
			case "start":
				base.startErr = errors.New("start failed")
			case "not running":
				base.holdStart = true
			}
			code, _ := Run([]string{"start", "myapp"})
			if code != 1 || system.started || app.started || app.checked != 0 {
				t.Fatalf("code %d system %v app %v", code, system.started, app.started)
			}
			if failure == "unknown" && (strings.Contains(h.out.String(), "[y/N]") || base.started) {
				t.Fatal("offered unknown dependency")
			}
		})
	}
}

func TestDependencySequencesStopAtMiddleFailure(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			initial := target.Stopped
			if action == "stop" {
				initial = target.Running
			}
			base, middle, leaf := node("base", initial), node("middle", initial, "base"), node("leaf", initial, "middle")
			_, calls, _, _ := dependencySetup(t, true, "yes\n", base, middle, leaf)
			selected := "leaf"
			expected := []string{"start base", "start middle"}
			if action == "start" {
				middle.startErr = errors.New("middle start failed")
			} else {
				selected = "base"
				middle.stopErr = errors.New("middle stop failed")
				expected = []string{"stop leaf", "stop middle"}
			}
			code, err := Run([]string{action, selected})
			if code != 1 || err == nil || !reflect.DeepEqual(*calls, expected) {
				t.Fatalf("%d %v %v", code, err, *calls)
			}
		})
	}
}

func TestDiamondStartsSharedDependencyOnlyOnce(t *testing.T) {
	_, calls, _, _ := dependencySetup(t, true, "yes\n", node("base", target.Stopped), node("left", target.Stopped, "base"), node("right", target.Stopped, "base"), node("leaf", target.Stopped, "left", "right"))
	code, err := Run([]string{"start", "leaf"})
	if code != 0 || err != nil || !reflect.DeepEqual(*calls, []string{"start base", "start left", "start right", "start leaf"}) {
		t.Fatalf("%d %v %v", code, err, *calls)
	}
}

func TestStopSkipsStoppedAndIndependentTargets(t *testing.T) {
	base, stopped, active, independent := node("base", target.Running), node("idle", target.Stopped, "base"), node("active", target.Running, "base"), node("other", target.Running)
	h, calls, _, _ := dependencySetup(t, true, "y\n", base, stopped, active, independent)
	code, err := Run([]string{"stop", "base"})
	if code != 0 || err != nil || !reflect.DeepEqual(*calls, []string{"stop active", "stop base"}) || independent.probes != 0 || strings.Contains(h.out.String(), "idle") {
		t.Fatalf("%d %v %v %s", code, err, *calls, h.out)
	}
}

// AS-122: no dependency configuration adds neither prompts nor changes to stop output.
func TestNoDependenciesPreservesSingleTargetStopOutput(t *testing.T) {
	h, _, _, _ := dependencySetup(t, true, "y\n", node("myapp", target.Running), node("other", target.Running))
	dependencyTerminal = func() bool { t.Fatal("unexpected terminal check"); return false }
	if code, err := Run([]string{"stop", "myapp"}); code != 0 || err != nil || h.out.String() != "myapp stop requested.\n" {
		t.Fatalf("%d %v %q", code, err, h.out.String())
	}
}

func TestStopDependentNoticeAgreesWithCount(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			nodes := []*dependencyFake{node("container", target.Running), node("myapp", target.Running, "container")}
			want := "myapp depends on container and is running.\n"
			if count == 2 {
				nodes = append(nodes, node("other", target.Running, "container"))
				want = "myapp and other depend on container and are running.\n"
			}
			h, calls, _, _ := dependencySetup(t, false, "", nodes...)
			code, _ := Run([]string{"stop", "container"})
			if code != 1 || len(*calls) != 0 || !strings.Contains(h.out.String(), want) {
				t.Fatalf("code %d calls %v output %s", code, *calls, h.out)
			}
		})
	}
}

func TestContainerSystemRunningBlocksEject(t *testing.T) {
	system, app := node("system", target.Running), node("myapp", target.Running, "system")
	system.kind = config.KindContainerSystem
	h, _, _, _ := dependencySetup(t, true, "", system, app)
	code, _ := Run([]string{"stop", "myapp", "--eject"})
	if code != 1 || h.ejected || !strings.Contains(h.out.String(), "system: running") {
		t.Fatalf("code %d ejected %v %s", code, h.ejected, h.out)
	}
}

func TestUnknownDependencyProbeErrorIsReturnedWithoutPrompt(t *testing.T) {
	base, app := node("base", target.Unknown), node("myapp", target.Stopped, "base")
	h, calls, _, _ := dependencySetup(t, true, "y\n", base, app)
	base.stateErr = errors.New("probe denied")
	code, err := Run([]string{"start", "myapp"})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "probe denied") || strings.Contains(h.out.String(), "[y/N]") || len(*calls) != 0 || app.checked != 0 {
		t.Fatalf("%d %v %v %s", code, err, *calls, h.out)
	}
}

func TestBuildContainerSystemTarget(t *testing.T) {
	entries, err := defaultBuildTargets(config.Config{Drive: "/Volumes/ExternalSSD", Targets: []config.Target{{Name: "system", Kind: config.KindContainerSystem, ContainerSystem: &config.ContainerSystem{DataPath: "/Volumes/ExternalSSD/container-data"}}}})
	if err != nil || len(entries) != 1 || entries[0].t.Kind() != config.KindContainerSystem || entries[0].t.MainPath() != "/Volumes/ExternalSSD/container-data" {
		t.Fatalf("%v %v", entries, err)
	}
}

// AS-114/115/116: all transitive dependents count, including partial and unknown states.
func TestStopDependencyAnswers(t *testing.T) {
	for _, tt := range []struct {
		name, answer   string
		terminal, want bool
	}{
		{"y", "y\n", true, true}, {"yes", "YES\n", true, true}, {"enter", "\n", true, false},
		{"no", "n\n", true, false}, {"other", "go\n", true, false}, {"EOF", "", true, false},
		{"partial EOF", "yes", true, false}, {"non terminal", "y\n", false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base, first, last := node("base", target.Running), node("first", target.Partial, "base"), node("last", target.Unknown, "first")
			h, calls, _, _ := dependencySetup(t, tt.terminal, tt.answer, base, first, last)
			code, err := Run([]string{"stop", "base"})
			if tt.want {
				if code != 0 || err != nil || !reflect.DeepEqual(*calls, []string{"stop last", "stop first", "stop base"}) {
					t.Fatalf("%d %v %v", code, err, *calls)
				}
			} else if code != 1 || len(*calls) != 0 || !strings.Contains(h.out.String(), "Not stopped. Stop these first: first, last\n") {
				t.Fatalf("%d %v %v %s", code, err, *calls, h.out)
			}
		})
	}
}

func TestStopDependencyFailurePreventsTargetAndEject(t *testing.T) {
	for _, failure := range []string{"stop error", "not stopped"} {
		t.Run(failure, func(t *testing.T) {
			base, app := node("base", target.Running), node("myapp", target.Running, "base")
			h, calls, _, _ := dependencySetup(t, true, "y\n", base, app)
			if failure == "stop error" {
				app.stopErr = errors.New("down failed")
			} else {
				app.holdStop = true
			}
			code, _ := Run([]string{"stop", "base", "--eject"})
			if code != 1 || base.stopped || h.ejected || !reflect.DeepEqual(*calls, []string{"stop myapp"}) {
				t.Fatalf("%d %v ejected %v", code, *calls, h.ejected)
			}
		})
	}
}

func TestStopDependenciesThenEject(t *testing.T) {
	h, calls, _, _ := dependencySetup(t, true, "y\n", node("base", target.Running), node("myapp", target.Running, "base"))
	code, err := Run([]string{"stop", "base", "--eject"})
	if code != 0 || err != nil || !h.ejected || !reflect.DeepEqual(*calls, []string{"stop myapp", "stop base"}) {
		t.Fatalf("%d %v %v eject %v", code, err, *calls, h.ejected)
	}
}

func TestDependencyGraphOrdersChainsAndIndependentTargets(t *testing.T) {
	_, _, _, entries := dependencySetup(t, false, "", node("leaf", target.Stopped, "middle"), node("independent", target.Stopped), node("middle", target.Stopped, "base"), node("base", target.Stopped), node("last", target.Stopped))
	g, err := newDependencyGraph(entries)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range g.order {
		names = append(names, e.t.Name())
	}
	if !reflect.DeepEqual(names, []string{"base", "middle", "leaf", "independent", "last"}) {
		t.Fatal(names)
	}
}

// Dashboard dependency confirmations are Slice 2: until then refuse rather than read TUI stdin.
func TestDashboardDependencyActionRefusesWithoutCLIPrompt(t *testing.T) {
	h, calls, cfg, entries := dependencySetup(t, true, "y\n", node("base", target.Stopped), node("myapp", target.Stopped, "base"))
	c := controller{cfg: cfg, entries: entries}
	ok, message := c.Run(ui.Start, "myapp")
	if ok || len(*calls) != 0 || strings.Contains(message, "[y/N]") || !strings.Contains(message, "Not started.") {
		t.Fatalf("%v %v %s %s", ok, *calls, message, h.out)
	}
}
