package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
)

// AS-117/118: prerequisite config position does not override dependencies; ready ties reverse config order.
func TestStopAllDependencyOrderAndEject(t *testing.T) {
	for _, eject := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "eject"}[eject], func(t *testing.T) {
			leaf, independent, base, last := node("leaf", target.Running, "base"), node("independent", target.Running), node("base", target.Running), node("last", target.Running)
			base.kind = config.KindContainerSystem
			h, calls, _, _ := dependencySetup(t, false, "", leaf, independent, base, last)
			dependencyTerminal = func() bool { t.Fatal("stop --all prompted"); return false }
			args := []string{"stop", "--all"}
			if eject {
				args = append(args, "--eject")
			}
			code, err := Run(args)
			if code != 0 || err != nil || h.ejected != eject || !reflect.DeepEqual(*calls, []string{"stop last", "stop independent", "stop leaf", "stop base"}) {
				t.Fatalf("%d %v %v eject %v\n%s", code, err, *calls, h.ejected, h.out)
			}
		})
	}
}

func TestStopAllFailureProtectsTransitiveDependencies(t *testing.T) {
	for _, failure := range []string{"error", "still running", "unknown"} {
		t.Run(failure, func(t *testing.T) {
			leaf, middle, base, other := node("leaf", target.Running, "middle"), node("middle", target.Running, "base"), node("base", target.Running), node("other", target.Running)
			base.kind = config.KindContainerSystem
			h, calls, _, _ := dependencySetup(t, true, "yes\n", leaf, middle, base, other)
			switch failure {
			case "error":
				leaf.stopErr = errors.New("down failed")
			case "still running":
				leaf.holdStop = true
			case "unknown":
				leaf.state = target.Unknown
				leaf.holdStop = true
				leaf.stateErr = errors.New("probe failed")
			}
			code, err := Run([]string{"stop", "--all", "--eject"})
			if code != 1 || err == nil || h.ejected || middle.stopped || base.stopped || !other.stopped || !reflect.DeepEqual(*calls, []string{"stop other", "stop leaf"}) {
				t.Fatalf("%d %v %v eject %v", code, err, *calls, h.ejected)
			}
			for _, name := range []string{"leaf", "middle", "base"} {
				if !strings.Contains(h.out.String(), name) {
					t.Fatalf("missing failure/protection %s: %s", name, h.out)
				}
			}
		})
	}
}

// A legacy stop_system target still exposes a partial aggregate state after StopStack.
type legacyDependencyStack struct {
	*dependencyFake
	systemUp bool
}

func (s *legacyDependencyStack) State() (target.State, error) {
	if s.state == target.Stopped && s.systemUp {
		return target.Partial, nil
	}
	return s.dependencyFake.State()
}
func (s *legacyDependencyStack) GuardState() (target.State, error) { return s.dependencyFake.State() }
func (s *legacyDependencyStack) StopStack() (bool, error)          { return s.dependencyFake.Stop() }
func (s *legacyDependencyStack) StopSystem() (bool, error) {
	if !s.systemUp {
		return false, nil
	}
	*s.calls = append(*s.calls, "system "+s.name)
	s.systemUp = false
	return true, nil
}
func (s *legacyDependencyStack) Stop() (bool, error) {
	_, err := s.StopStack()
	if err != nil {
		return false, err
	}
	return s.StopSystem()
}

// AS-117/119 mixed mode: target dependency order first, then the unchanged legacy system pass.
func TestStopAllDependenciesWithLegacySystemPass(t *testing.T) {
	stack, db := node("stack", target.Running, "db"), node("db", target.Running)
	h, calls, cfg, entries := dependencySetup(t, false, "", stack, db)
	legacy := &legacyDependencyStack{dependencyFake: stack, systemUp: true}
	cfg.Targets[0].Container = &config.Container{StopSystem: true}
	entries[0].cfg = cfg.Targets[0]
	entries[0].t = legacy
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	code, err := Run([]string{"stop", "--all", "--eject"})
	if code != 0 || err != nil || !h.ejected || legacy.systemUp || !reflect.DeepEqual(*calls, []string{"stop stack", "stop db", "system stack"}) {
		t.Fatalf("%d %v %v eject %v\n%s", code, err, *calls, h.ejected, h.out)
	}
	if !strings.Contains(h.out.String(), "stack: container system stop requested.") {
		t.Fatal(h.out)
	}
}

func TestStopAllProtectsLegacySystemOfFailedDependent(t *testing.T) {
	leaf, base := node("leaf", target.Running, "base"), node("base", target.Running)
	h, calls, cfg, entries := dependencySetup(t, false, "", leaf, base)
	legacy := &legacyDependencyStack{dependencyFake: base, systemUp: true}
	cfg.Targets[1].Container = &config.Container{StopSystem: true}
	entries[1].cfg = cfg.Targets[1]
	entries[1].t = legacy
	loadConfig = func(string) (config.Config, error) { return cfg, nil }
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	leaf.stopErr = errors.New("down failed")
	code, _ := Run([]string{"stop", "--all", "--eject"})
	if code != 1 || h.ejected || !legacy.systemUp || !reflect.DeepEqual(*calls, []string{"stop leaf"}) {
		t.Fatalf("%d %v system up %v", code, *calls, legacy.systemUp)
	}
}

func TestStopSystemDeprecationInListAndStatus(t *testing.T) {
	for _, command := range []string{"list", "status"} {
		t.Run(command, func(t *testing.T) {
			h, _, cfg, entries := dependencySetup(t, false, "", node("first", target.Stopped), node("second", target.Stopped), node("normal", target.Stopped))
			for i := 0; i < 2; i++ {
				cfg.Targets[i].Container = &config.Container{StopSystem: true}
				entries[i].cfg = cfg.Targets[i]
			}
			loadConfig = func(string) (config.Config, error) { return cfg, nil }
			buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
			code, err := Run([]string{command})
			if code != 0 || err != nil {
				t.Fatalf("%d %v", code, err)
			}
			note := "stop_system is deprecated; add a container-system target and depends_on (see README)."
			if strings.Count(h.out.String(), note) != 2 || !strings.Contains(h.out.String(), "first: "+note) || !strings.Contains(h.out.String(), "second: "+note) || strings.Contains(h.out.String(), "normal: "+note) {
				t.Fatal(h.out)
			}
		})
	}
}

// AS-122: preserve all legacy stop output, including the single shared-system line.
func TestLegacyStopAllOutputUnchanged(t *testing.T) {
	h := setupStacks(t, &sharedSystem{up: true, running: map[string]bool{"stack-a": true, "stack-b": true}})
	code, err := Run([]string{"stop", "--all"})
	want := "stack-b stop requested.\nstack-a stop requested.\nstack-b: container system stop requested.\n"
	if code != 0 || err != nil || h.out.String() != want {
		t.Fatalf("%d %v output %q", code, err, h.out.String())
	}
}

type systemPassSentinel struct {
	*dependencyFake
	legacyCalls int
}

func (s *systemPassSentinel) StopStack() (bool, error)  { return s.dependencyFake.Stop() }
func (s *systemPassSentinel) StopSystem() (bool, error) { s.legacyCalls++; return false, nil }

func TestExplicitSystemDisablesLegacyPass(t *testing.T) {
	system, stack := node("system", target.Running), node("stack", target.Running, "system")
	system.kind = config.KindContainerSystem
	_, calls, cfg, entries := dependencySetup(t, false, "", system, stack)
	sentinel := &systemPassSentinel{dependencyFake: stack}
	entries[1].t = sentinel
	buildTargets = func(config.Config) ([]entry, error) { return entries, nil }
	code, err := runStopAll(cfg, entries, false, nil)
	if code != 0 || err != nil || sentinel.legacyCalls != 0 || !reflect.DeepEqual(*calls, []string{"stop stack", "stop system"}) {
		t.Fatalf("%d %v calls %v legacy calls %d", code, err, *calls, sentinel.legacyCalls)
	}
}
