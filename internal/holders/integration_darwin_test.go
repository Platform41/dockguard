package holders

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/platform"
)

func TestStopOwnChild(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root-owned holders are never offered")
	}
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%v", force), func(t *testing.T) {
			dir := t.TempDir()
			script := "printf 'ready\\n'; exec /bin/sleep 60"
			if force {
				// Ignored dispositions survive exec, so only the sleep process is started.
				script = "trap '' TERM; " + script
			}
			child := exec.Command("/bin/sh", "-c", script)
			child.Dir = dir
			out, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			pid := child.Process.Pid
			reaped := false
			t.Cleanup(func() {
				if !reaped {
					_ = child.Process.Kill()
					_ = child.Wait()
				}
			})
			ready, err := bufio.NewReader(out).ReadString('\n')
			if err != nil || ready != "ready\n" {
				t.Fatalf("child readiness %q: %v", ready, err)
			}
			deadline := time.Now().Add(time.Second)
			// The readiness write precedes exec. Wait for the child to have installed sleep.
			var process platform.Process
			for {
				process, err = platform.InspectProcess(pid)
				if err == nil && process.Name == "sleep" {
					break
				}
				if !time.Now().Before(deadline) {
					t.Fatalf("child process = %+v, %v", process, err)
				}
				time.Sleep(time.Millisecond)
			}

			s := Service{
				PID: os.Getpid(), UID: os.Geteuid(), Inspect: platform.InspectProcess,
				Ancestors: func(int) ([]int, error) { return nil, nil },
				Now:       time.Now, Sleep: time.Sleep,
				Scan: func(path string) ([]platform.Holder, error) {
					// Use a directory selection, restricted to the child PID, so a
					// temporary folder can stand in for a mounted drive.
					cmd := exec.Command("/usr/sbin/lsof", "-a", "-p", strconv.Itoa(pid), "+d", path, "-FpcuLR")
					out, err := cmd.CombinedOutput()
					if code, ok := err.(*exec.ExitError); ok && code.ExitCode() == 1 && len(bytes.TrimSpace(out)) == 0 {
						return nil, nil
					}
					if err != nil {
						return nil, fmt.Errorf("lsof: %w: %s", err, out)
					}
					fields := strings.Split(string(out), "\n")
					if !hasField(fields, "p"+strconv.Itoa(pid)) || !hasField(fields, "csleep") ||
						!hasField(fields, "u"+strconv.Itoa(process.UID)) {
						return nil, fmt.Errorf("unexpected child holder: %s", out)
					}
					return []platform.Holder{{Name: process.Name, PID: pid, UID: process.UID}}, nil
				},
				Signal: func(gotPID int, sig syscall.Signal) error {
					if gotPID != pid {
						t.Fatalf("attempt to signal PID %d instead of child %d", gotPID, pid)
					}
					// Keep the child unreaped until the sequence finishes so its
					// PID cannot be reassigned while the real signaller is active.
					return platform.Signal(pid, sig)
				},
			}
			list, err := s.Scan(dir)
			if err != nil || len(list) != 1 {
				t.Fatalf("child holder scan = %v, %v", list, err)
			}
			candidates := s.Classify(list)
			if !candidates[0].CanStop() {
				t.Fatalf("child classification = %v", candidates)
			}
			questions := 0
			result, err := s.Stop(dir, candidates[0], func(string, bool) bool { questions++; return true })
			want, count := Stopped, 1
			if force {
				want, count = ForceStopped, 2
			}
			if err != nil || result.Status != want || questions != count {
				t.Fatalf("result = %+v, error = %v, questions = %d", result, err, questions)
			}
			// A stopped result can also mean release. Bound the exit check
			// independently so that a regression cannot leave a running child.
			done := make(chan error, 1)
			go func() { done <- child.Wait() }()
			select {
			case err = <-done:
				reaped = true
			case <-time.After(time.Second):
				_ = child.Process.Kill()
				<-done
				reaped = true
				t.Fatal("child did not exit")
			}
			got, ok := err.(*exec.ExitError)
			wantSignal := syscall.SIGTERM
			if force {
				wantSignal = syscall.SIGKILL
			}
			if !ok || got.Sys().(syscall.WaitStatus).Signal() != wantSignal {
				t.Fatalf("child exit = %v, want %v", err, wantSignal)
			}
		})
	}
}

func hasField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}
