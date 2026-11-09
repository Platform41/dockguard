package dockerdesktop

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMain makes the process check report a running Desktop by default, so these tests exercise
// the CLI path whatever is running on the test machine.
func TestMain(m *testing.M) {
	desktopProcesses = func() (bool, error) { return true, nil }
	os.Exit(m.Run())
}

func TestIsRunningSkipsTheCLIWhenNoDesktopProcessExists(t *testing.T) {
	origProc, origStatus, origLook := desktopProcesses, runStatus, lookPath
	t.Cleanup(func() { desktopProcesses, runStatus, lookPath = origProc, origStatus, origLook })
	desktopProcesses = func() (bool, error) { return false, nil }
	runStatus = func(string, ...string) (string, error) {
		t.Fatal("docker desktop status ran although no Desktop process exists")
		return "", nil
	}

	running, err := IsRunning()
	if running || err != nil {
		t.Fatalf("IsRunning() = %v, %v; want stopped", running, err)
	}
}

func TestIsRunningAsksTheCLIWhenTheProcessCheckFails(t *testing.T) {
	origProc, origStatus, origLook := desktopProcesses, runStatus, lookPath
	t.Cleanup(func() { desktopProcesses, runStatus, lookPath = origProc, origStatus, origLook })
	desktopProcesses = func() (bool, error) { return false, errors.New("pgrep: exit 3") }
	lookPath = func(file string) (string, error) { return "/usr/local/bin/" + file, nil }
	asked := false
	runStatus = func(string, ...string) (string, error) { asked = true; return "Name Value\nStatus running\n", nil }

	if running, err := IsRunning(); !running || err != nil || !asked {
		t.Fatalf("IsRunning() = %v, %v; asked CLI = %v", running, err, asked)
	}
}

func TestStartRunsDockerDesktopStart(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		if file != "docker" {
			t.Fatalf("lookPath called with %q", file)
		}
		return "/usr/local/bin/" + file, nil
	}

	called := false
	runCmd = func(name string, args ...string) error {
		called = true
		if name != "docker" {
			t.Fatalf("runCmd name = %q", name)
		}
		if len(args) != 2 || args[0] != "desktop" || args[1] != "start" {
			t.Fatalf("runCmd args = %#v", args)
		}
		return nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		t.Fatal("runStatus should not be called when already-running guard is disabled")
		return "", nil
	}

	if err := Start(StartOptions{}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !called {
		t.Fatal("runCmd was not called")
	}
}

func TestStartFailsWhenDockerCLIIsMissing(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	runCmd = func(name string, args ...string) error {
		t.Fatal("runCmd should not be called when docker is missing")
		return nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		t.Fatal("runStatus should not be called when docker is missing")
		return "", nil
	}

	err := Start(StartOptions{})
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestStartReturnsWrappedCommandError(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}

	runCmd = func(name string, args ...string) error {
		return errors.New("exit status 1: unsupported command")
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "", nil
	}

	err := Start(StartOptions{
		RequireCLIStartSupport: true,
	})
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestStartReturnsUnsupportedDesktopCLIHints(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}

	runCmd = func(name string, args ...string) error {
		return errors.New("unknown command \"desktop\" for \"docker\"")
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "", nil
	}
	if err := Start(StartOptions{
		RequireCLIStartSupport: true,
	}); err == nil {
		t.Fatal("Start() error = nil, want error")
	} else if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Start() error = %q, want unsupported hint", err)
	}
}

func TestStartFailsWhenAlreadyRunning(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}

	runCmd = func(name string, args ...string) error {
		t.Fatal("runCmd should not be called when Docker Desktop is already running")
		return nil
	}

	runStatus = func(name string, args ...string) (string, error) {
		return "Name Value\nStatus running\n", nil
	}

	err := Start(StartOptions{
		FailIfAlreadyRunning: true,
	})
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestIsRunningReturnsTrueWhenStatusRunning(t *testing.T) {
	originalLookPath := lookPath
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Status running\n", nil
	}

	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if !running {
		t.Fatal("IsRunning() = false, want true")
	}
}

func TestIsRunningAcceptsStatusWithColon(t *testing.T) {
	originalLookPath := lookPath
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Status: running\n", nil
	}

	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if !running {
		t.Fatal("IsRunning() = false, want true")
	}
}

func TestIsRunningReturnsFalseWhenStatusStopped(t *testing.T) {
	originalLookPath := lookPath
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Name Value\nStatus stopped\n", nil
	}

	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if running {
		t.Fatal("IsRunning() = true, want false")
	}
}

func TestIsRunningReturnsFalseWhenStatusCommandErrorsForStopped(t *testing.T) {
	originalLookPath := lookPath
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Could not retrieve status. Is Docker Desktop running?\nYou can start Docker Desktop by running 'docker desktop start'.\n", errors.New("exit status 1")
	}

	running, err := IsRunning()
	if err != nil {
		t.Fatalf("IsRunning() error = %v", err)
	}
	if running {
		t.Fatal("IsRunning() = true, want false")
	}
}

func TestIsRunningReturnsErrorWhenStatusMissing(t *testing.T) {
	originalLookPath := lookPath
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Name Value\nState running\n", nil
	}

	if _, err := IsRunning(); err == nil {
		t.Fatal("IsRunning() error = nil, want error")
	}
}

func TestStopRunsDockerDesktopStop(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		if file != "docker" {
			t.Fatalf("lookPath called with %q", file)
		}
		return "/usr/local/bin/" + file, nil
	}

	called := false
	runStatus = func(name string, args ...string) (string, error) {
		return "Status running\n", nil
	}
	runCmd = func(name string, args ...string) error {
		called = true
		if name != "docker" {
			t.Fatalf("runCmd name = %q", name)
		}
		if len(args) != 2 || args[0] != "desktop" || args[1] != "stop" {
			t.Fatalf("runCmd args = %#v", args)
		}
		return nil
	}

	requested, err := Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if !requested {
		t.Fatal("Stop() requested = false, want true")
	}

	if !called {
		t.Fatal("runCmd was not called")
	}
}

func TestStopReturnsNilWhenAlreadyStopped(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		return "Status stopped\n", nil
	}

	runCmd = func(name string, args ...string) error {
		t.Fatal("runCmd should not be called when Docker Desktop is already stopped")
		return nil
	}

	requested, err := Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
	if requested {
		t.Fatal("Stop() requested = true, want false")
	}
}

func TestStopFailsWhenDockerCLIIsMissing(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	originalRunStatus := runStatus
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
		runStatus = originalRunStatus
	})

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	runCmd = func(name string, args ...string) error {
		t.Fatal("runCmd should not be called when docker is missing")
		return nil
	}
	runStatus = func(name string, args ...string) (string, error) {
		t.Fatal("runStatus should not be called when docker is missing")
		return "", nil
	}

	if _, err := Stop(); err == nil {
		t.Fatal("Stop() error = nil, want error")
	}
}
