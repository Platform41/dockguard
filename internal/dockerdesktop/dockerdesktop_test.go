package dockerdesktop

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/config"
)

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

	if err := Start(config.Config{}); err != nil {
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

	err := Start(config.Config{})
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

	err := Start(config.Config{
		DockerDesktopConfig: config.DockerDesktopConfig{
			RequireCLIStartSupport: true,
		},
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
	if err := Start(config.Config{
		DockerDesktopConfig: config.DockerDesktopConfig{
			RequireCLIStartSupport: true,
		},
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

	err := Start(config.Config{
		DockerDesktopConfig: config.DockerDesktopConfig{
			FailIfAlreadyRunning: true,
		},
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
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
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
		if len(args) != 2 || args[0] != "desktop" || args[1] != "stop" {
			t.Fatalf("runCmd args = %#v", args)
		}
		return nil
	}

	if err := Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if !called {
		t.Fatal("runCmd was not called")
	}
}

func TestStopReturnsNilWhenAlreadyStopped(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/" + file, nil
	}

	runCmd = func(name string, args ...string) error {
		return errors.New("exit status 1: Docker Desktop is not running")
	}

	if err := Stop(); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}
}

func TestStopFailsWhenDockerCLIIsMissing(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
	})

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	runCmd = func(name string, args ...string) error {
		t.Fatal("runCmd should not be called when docker is missing")
		return nil
	}

	if err := Stop(); err == nil {
		t.Fatal("Stop() error = nil, want error")
	}
}
