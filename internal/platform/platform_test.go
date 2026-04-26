package platform

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestEjectVolumeRunsDiskutilEject(t *testing.T) {
	originalLookPath := lookPath
	originalRunCommand := runCommand
	originalSleep := sleep
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCommand = originalRunCommand
		sleep = originalSleep
	})
	sleep = func(time.Duration) {}

	lookPath = func(file string) (string, error) {
		if file != "diskutil" {
			t.Fatalf("lookPath called with %q", file)
		}
		return "/usr/sbin/diskutil", nil
	}

	called := false
	runCommand = func(name string, args ...string) error {
		called = true
		if name != "diskutil" {
			t.Fatalf("runCommand name = %q", name)
		}
		if len(args) != 2 || args[0] != "eject" || args[1] != "/Volumes/DockerSSD" {
			t.Fatalf("runCommand args = %#v", args)
		}
		return nil
	}

	if err := EjectVolume("/Volumes/DockerSSD"); err != nil {
		t.Fatalf("EjectVolume() error = %v", err)
	}

	if !called {
		t.Fatal("runCommand was not called")
	}
}

func TestEjectVolumeFailsWhenMissingMountPath(t *testing.T) {
	if err := EjectVolume(""); err == nil {
		t.Fatal("EjectVolume() error = nil, want error")
	}
}

func TestEjectVolumeFailsWhenDiskutilIsMissing(t *testing.T) {
	originalLookPath := lookPath
	originalRunCommand := runCommand
	originalSleep := sleep
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCommand = originalRunCommand
		sleep = originalSleep
	})
	sleep = func(time.Duration) {}

	lookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}

	runCommand = func(name string, args ...string) error {
		t.Fatal("runCommand should not be called when diskutil is missing")
		return nil
	}

	if err := EjectVolume("/Volumes/DockerSSD"); err == nil {
		t.Fatal("EjectVolume() error = nil, want error")
	}
}

func TestEjectVolumeWrapsEjectError(t *testing.T) {
	originalLookPath := lookPath
	originalRunCommand := runCommand
	originalSleep := sleep
	t.Cleanup(func() {
		lookPath = originalLookPath
		runCommand = originalRunCommand
		sleep = originalSleep
	})
	sleep = func(time.Duration) {}

	lookPath = func(file string) (string, error) {
		return "/usr/sbin/diskutil", nil
	}

	runCommand = func(name string, args ...string) error {
		return errors.New("resource busy")
	}

	err := EjectVolume("/Volumes/DockerSSD")
	if err == nil {
		t.Fatal("EjectVolume() error = nil, want error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "volume is busy") {
		t.Fatalf("EjectVolume() error = %q, want busy-volume guidance", err)
	}
}
