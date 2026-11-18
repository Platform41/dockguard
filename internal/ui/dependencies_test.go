package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/platform41/dockguard/internal/target"
)

type dependencyCtl struct {
	*fakeCtl
	plan      Confirmation
	planErr   error
	plans     []call
	confirmed []call
	approval  Confirmation
}

func (f *dependencyCtl) Plan(a Action, name string) (Confirmation, error) {
	f.plans = append(f.plans, call{a, name})
	return f.plan, f.planErr
}
func (f *dependencyCtl) RunConfirmed(a Action, name string, p Confirmation) (bool, string) {
	f.confirmed = append(f.confirmed, call{a, name})
	f.approval = p
	return true, "done"
}
func dependencyModel(f *dependencyCtl) Model {
	next, _ := New(f).Update(snapshotMsg(f.snap))
	return next.(Model)
}
func applyPlan(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no plan command")
	}
	next, out := m.Update(cmd())
	return next.(Model), out
}

func TestViewShowsDependencyAfterKind(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running, target.Stopped)}
	f.snap.Rows[1].Kind = "app"
	f.snap.Rows[1].DependsOn = []string{"docker"}
	view := loaded(f).View()
	if !strings.Contains(view, "app  → docker") {
		t.Fatal(view)
	}
}

// AS-121: planned dependency actions reuse pending; no input besides a fresh y confirms them.
func TestDependencyConfirmationCancelAndAccept(t *testing.T) {
	for _, action := range []Action{Start, Stop} {
		for _, answer := range []string{"n", "Y", "y", "enter", "q"} {
			t.Run(action.String()+answer, func(t *testing.T) {
				f := &dependencyCtl{fakeCtl: &fakeCtl{snap: snapshot(target.Stopped, target.Running)}, plan: Confirmation{Question: "Start docker first?", Detail: "xcode needs docker, which is stopped.", Targets: []string{"docker"}}}
				key := "s"
				if action == Stop {
					key = "x"
					f.plan = Confirmation{Question: "Stop them first?", Detail: "xcode depends on docker and is running.", Targets: []string{"xcode"}}
				}
				m := dependencyModel(f)
				m.selected = 1
				m, cmd := press(m, key)
				if len(f.calls) != 0 || len(f.confirmed) != 0 {
					t.Fatal("action before plan")
				}
				m, cmd = applyPlan(t, m, cmd)
				if cmd != nil || m.confirming == nil || !strings.Contains(m.View(), f.plan.Question) || !strings.Contains(m.View(), f.plan.Detail) {
					t.Fatal(m.View())
				}
				m, cmd = press(m, answer)
				if answer == "y" || answer == "Y" {
					if cmd == nil {
						t.Fatal("yes did not dispatch")
					}
					cmd()
					if !reflect.DeepEqual(f.confirmed, []call{{action, "xcode"}}) || !reflect.DeepEqual(f.approval.Targets, f.plan.Targets) {
						t.Fatalf("confirmed %+v approval %+v", f.confirmed, f.approval)
					}
				} else if cmd != nil || len(f.confirmed) != 0 || m.confirming != nil {
					t.Fatal("no did not cancel")
				}
			})
		}
	}
}

func TestUnknownDependencyIsNotOffered(t *testing.T) {
	f := &dependencyCtl{fakeCtl: &fakeCtl{snap: snapshot(target.Unknown, target.Stopped)}, planErr: errors.New("dependency docker state unknown: probe denied")}
	m := dependencyModel(f)
	m.selected = 1
	m, cmd := press(m, "s")
	m, _ = applyPlan(t, m, cmd)
	if m.confirming != nil || len(f.calls) != 0 || len(f.confirmed) != 0 || !strings.Contains(m.View(), "probe denied") {
		t.Fatal(m.View())
	}
}

func TestDependencyPlanWaitsForInFlightRefresh(t *testing.T) {
	f := &dependencyCtl{fakeCtl: &fakeCtl{snap: snapshot(target.Stopped)}, plan: Confirmation{Question: "Start base first?", Targets: []string{"base"}}}
	m := dependencyModel(f)
	m.refreshing = true
	m, cmd := press(m, "s")
	if cmd != nil || m.queued == nil || len(f.plans) != 0 {
		t.Fatal("planning overlapped refresh")
	}
	next, cmd := m.Update(snapshotMsg(f.snap))
	m, cmd = applyPlan(t, next.(Model), cmd)
	if m.confirming == nil || cmd != nil || len(f.plans) != 1 {
		t.Fatal("queued plan lost")
	}
	m, cmd = press(m, "y")
	cmd()
	if len(f.confirmed) != 1 {
		t.Fatal("confirmation lost")
	}
}

func TestPastedStartAndYesCannotApproveDependencies(t *testing.T) {
	f := &dependencyCtl{fakeCtl: &fakeCtl{snap: snapshot(target.Stopped)}, plan: Confirmation{Question: "Start base first?", Targets: []string{"base"}}}
	m, cmd := press(dependencyModel(f), "sy")
	m, _ = applyPlan(t, m, cmd)
	if m.confirming == nil || len(f.confirmed) != 0 || len(f.calls) != 0 {
		t.Fatal("paste approved an action")
	}
}

func TestPlanWithNoStartDependenciesRunsNamedTarget(t *testing.T) {
	f := &dependencyCtl{fakeCtl: &fakeCtl{snap: snapshot(target.Stopped)}}
	m, cmd := press(dependencyModel(f), "s")
	m, cmd = applyPlan(t, m, cmd)
	if m.confirming != nil || cmd == nil {
		t.Fatal("unnecessary confirmation")
	}
	cmd()
	if !reflect.DeepEqual(f.calls, []call{{Start, "docker"}}) || len(f.confirmed) != 0 {
		t.Fatal(f.calls)
	}
}
