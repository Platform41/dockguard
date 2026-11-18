package app

import (
	"reflect"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
)

func TestDashboardPlansAndRunsConfirmedDependencyActions(t *testing.T) {
	for _, action := range []ui.Action{ui.Start, ui.Stop} {
		t.Run(action.String(), func(t *testing.T) {
			initial := target.Stopped
			selected := "myapp"
			if action == ui.Stop {
				initial = target.Running
				selected = "base"
			}
			_, calls, cfg, entries := dependencySetup(t, true, "", node("base", initial), node("myapp", initial, "base"))
			dependencyTerminal = func() bool { t.Fatal("dashboard read terminal input"); return false }
			c := controller{cfg: cfg, entries: entries}
			p, err := c.Plan(action, selected)
			if err != nil || p.Question == "" || len(*calls) != 0 {
				t.Fatalf("plan %+v %v calls %v", p, err, *calls)
			}
			ok, out := c.RunConfirmed(action, selected, p)
			want := []string{"start base", "start myapp"}
			if action == ui.Stop {
				want = []string{"stop myapp", "stop base"}
			}
			if !ok || !reflect.DeepEqual(*calls, want) || strings.Contains(out, "[y/N]") {
				t.Fatalf("%v %v %s", ok, *calls, out)
			}
		})
	}
}

func TestDashboardChangedDependenciesRequireNewConsent(t *testing.T) {
	base, first, second := node("base", target.Running), node("first", target.Running, "base"), node("second", target.Stopped, "base")
	_, calls, cfg, entries := dependencySetup(t, true, "", base, first, second)
	c := controller{cfg: cfg, entries: entries}
	p, err := c.Plan(ui.Stop, "base")
	if err != nil {
		t.Fatal(err)
	}
	second.state = target.Running
	ok, out := c.RunConfirmed(ui.Stop, "base", p)
	if ok || len(*calls) != 0 || !strings.Contains(out, "Not stopped.") {
		t.Fatalf("%v %v %s", ok, *calls, out)
	}
}

func TestDashboardSnapshotIncludesDependencies(t *testing.T) {
	_, _, cfg, entries := dependencySetup(t, false, "", node("base", target.Running), node("myapp", target.Stopped, "base"))
	s := (&controller{cfg: cfg, entries: entries}).Snapshot()
	if !reflect.DeepEqual(s.Rows[1].DependsOn, []string{"base"}) {
		t.Fatalf("%+v", s.Rows[1])
	}
}
