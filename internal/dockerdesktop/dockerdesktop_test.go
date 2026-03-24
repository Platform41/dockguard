package dockerdesktop

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/platform41/dockguard/internal/config"
)

func TestStartRunsDockerDesktopStart(t *testing.T) {
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
		return "/usr/local/bin/docker", nil
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

	err := Start(config.Config{})
	if err == nil {
		t.Fatal("Start() error = nil, want error")
	}
}

func TestStartReturnsWrappedCommandError(t *testing.T) {
	originalLookPath := lookPath
	originalRunCmd := runCmd
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCmd = originalRunCmd
	})

	lookPath = func(file string) (string, error) {
		return "/usr/local/bin/docker", nil
	}

	runCmd = func(name string, args ...string) error {
		return errors.New("exit status 1: unsupported command")
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
