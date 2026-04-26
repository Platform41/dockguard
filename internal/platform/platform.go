package platform

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const TargetOS = "darwin"

var (
	lookPath   = exec.LookPath
	runCommand = runDiskutilCommand
	sleep      = time.Sleep
)

func EjectVolume(mountPath string) error {
	if mountPath == "" {
		return fmt.Errorf("missing external mount path")
	}

	if _, err := lookPath("diskutil"); err != nil {
		return fmt.Errorf("diskutil not found in PATH: %w", err)
	}

	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := runCommand("diskutil", "eject", mountPath)
		if err == nil {
			return nil
		}

		lastErr = err
		if !isBusyUnmountError(err) {
			return fmt.Errorf("eject external mount %q: %w", mountPath, err)
		}

		if attempt < maxAttempts {
			sleep(1 * time.Second)
		}
	}

	return fmt.Errorf(
		"eject external mount %q failed after %d attempts: %w\nvolume is busy; close Finder previews/windows for this drive, ensure terminals are not inside the mount, and release file locks (for example QuickLook on Docker.raw), then retry",
		mountPath,
		maxAttempts,
		lastErr,
	)
}

func isBusyUnmountError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "could not be unmounted") {
		return true
	}
	if strings.Contains(message, "resource busy") {
		return true
	}
	if strings.Contains(message, "device busy") {
		return true
	}

	return false
}

func runDiskutilCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}

	return nil
}
