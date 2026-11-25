package holders

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/platform"
)

func TestClassification(t *testing.T) {
	marker := time.Unix(100, 1)
	for _, tt := range []struct {
		name, path, user, hint string
		pid, uid               int
		class                  Class
	}{
		{"root", "/sbin/launchd", "root", "doesn't use sudo", 1, 0, OtherUser},
		{"editor", "/usr/local/bin/editor", "502", "owned by 502", 10, 502, OtherUser},
		{"zsh", "/bin/zsh", "501", "relaunch DockGuard", 11, 501, Self},
		{"dockguard", "/tmp/dockguard", "501", "relaunch DockGuard", 12, 501, Self},
		{"mds", "/System/Library/Spotlight/mds", "501", "Spotlight", 13, 501, Protected},
		{"mdworker_shared", "/usr/libexec/mdworker_shared", "501", "Spotlight", 14, 501, Protected},
		{"mds", "/tmp/mds", "501", "", 15, 501, Stoppable},
		{"Finder", "/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder", "501", "", 16, 501, Stoppable},
	} {
		t.Run(tt.path, func(t *testing.T) {
			s := Service{PID: 12, UID: 501, Ancestors: func(int) ([]int, error) { return []int{11}, nil },
				Inspect: func(int) (platform.Process, error) {
					return platform.Process{Name: tt.name, Path: tt.path, UID: tt.uid, Started: marker}, nil
				}}
			got := s.Classify([]platform.Holder{{Name: tt.name, PID: tt.pid, UID: tt.uid, User: tt.user}})
			if got[0].Class != tt.class || !strings.Contains(got[0].String(), tt.hint) {
				t.Fatalf("classification = %v", got)
			}
		})
	}
}

func TestStopSequence(t *testing.T) {
	for _, tt := range []struct {
		name, mode string
		answers    []bool
		want       Status
		signals    []syscall.Signal
	}{
		{"AS126", "ignore", []bool{false}, Declined, nil},
		{"AS127", "quit", []bool{true}, Stopped, []syscall.Signal{syscall.SIGTERM}},
		{"AS128", "ignore", []bool{true, true}, ForceStopped, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}},
		{"AS129", "ignore", []bool{true, false}, Declined, []syscall.Signal{syscall.SIGTERM}},
		{"AS130-exited", "exited", []bool{true}, Gone, nil},
		{"AS130-reused", "reused", []bool{true}, Gone, nil},
		{"AS131", "release", []bool{true}, Stopped, []syscall.Signal{syscall.SIGTERM}},
		{"AS135", "EPERM", []bool{true}, Failed, []syscall.Signal{syscall.SIGTERM}},
		{"AS146", "finder", []bool{true}, Stopped, []syscall.Signal{syscall.SIGTERM}},
		{"AS152", "reuse-on-force", []bool{true, true}, Gone, []syscall.Signal{syscall.SIGTERM}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mount := "/Volumes/ExternalSSD"
			holder := platform.Holder{Name: "editor", PID: 42, UID: 501, User: "501"}
			if tt.mode == "finder" {
				holder.Name = "Finder"
			}
			p := platform.Process{Name: holder.Name, UID: holder.UID, Path: filepath.Join("/test", holder.Name), Started: time.Unix(10, 1)}
			if tt.mode == "finder" {
				p.Path = "/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder"
			}
			current := p.Started
			var sent []syscall.Signal
			var waitScans []time.Duration
			var checks int
			gone, holding := false, true
			question := 0
			s := Service{
				PID: 100, UID: holder.UID,
				Ancestors: func(int) ([]int, error) { return nil, nil },
				Now:       func() time.Time { return current },
				Sleep: func(d time.Duration) {
					current = current.Add(d)
					if tt.mode == "reuse-on-force" && current.Sub(time.Unix(10, 1)) == 4*time.Second {
						p.Started = p.Started.Add(time.Microsecond)
					}
				},
				Scan: func(string) ([]platform.Holder, error) {
					if len(sent) == 1 && question == 1 {
						waitScans = append(waitScans, current.Sub(time.Unix(10, 1)))
					}
					if holding {
						return []platform.Holder{holder}, nil
					}
					return nil, nil
				},
				Inspect: func(int) (platform.Process, error) {
					if gone {
						return platform.Process{}, os.ErrProcessDone
					}
					return p, nil
				},
				Signal: func(pid int, sig syscall.Signal) error {
					if pid != holder.PID {
						t.Fatalf("wrong PID %d", pid)
					}
					if sig == 0 {
						checks++
						if gone {
							return syscall.ESRCH
						}
						return nil
					}
					sent = append(sent, sig)
					if tt.mode == "EPERM" {
						return syscall.EPERM
					}
					if sig == syscall.SIGKILL || tt.mode == "quit" || tt.mode == "finder" {
						gone, holding = true, false
					}
					if tt.mode == "release" {
						holding = false
					}
					return nil
				},
			}
			before := s.Classify([]platform.Holder{holder})
			got, err := s.Stop(mount, before[0], func(q string, yesDefault bool) bool {
				if question >= len(tt.answers) {
					t.Fatalf("unexpected question %q", q)
				}
				if yesDefault != (question == 0) {
					t.Errorf("wrong default for %q", q)
				}
				if question == 1 && !strings.Contains(q, "Unsaved work will be lost.") {
					t.Errorf("missing warning in %q", q)
				}
				if tt.mode == "finder" && !strings.Contains(q, "macOS relaunches Finder") {
					t.Errorf("missing Finder hint in %q", q)
				}
				if tt.mode == "exited" {
					gone = true
				}
				if tt.mode == "reused" {
					p.Started = p.Started.Add(time.Microsecond)
				}
				answer := tt.answers[question]
				question++
				return answer
			})
			if err != nil || got.Status != tt.want || !reflect.DeepEqual(sent, tt.signals) || question != len(tt.answers) {
				t.Fatalf("result=%+v error=%v signals=%v questions=%d", got, err, sent, question)
			}
			if tt.mode == "EPERM" && !errors.Is(got.Err, syscall.EPERM) {
				t.Errorf("result error = %v", got.Err)
			}
			if question == 2 {
				want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second}
				if !reflect.DeepEqual(waitScans, want) || checks < 20 {
					t.Errorf("wait scans %v, existence checks %d", waitScans, checks)
				}
			}
		})
	}
}
