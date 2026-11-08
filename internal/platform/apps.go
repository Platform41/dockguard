package platform

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Swappable for tests.
var (
	runApp   = runAppCommand
	runPgrep = runPgrepCommand
)

// OpenApp opens a macOS app by name, as `open -a` does.
func OpenApp(app string) error {
	if err := runApp("open", "-a", app); err != nil {
		return fmt.Errorf("open %s: %w", app, err)
	}
	return nil
}

// QuitApp asks a macOS app to quit, so it can save first. It never force-quits.
func QuitApp(app string) error {
	script := fmt.Sprintf("quit app %q", app)
	if err := runApp("osascript", "-e", script); err != nil {
		return fmt.Errorf("quit %s: %w", app, err)
	}
	return nil
}

// ProcessRunning reports whether a process with exactly this name is running. pgrep exits 0 when
// it finds one and 1 when it finds none; anything else is an error, never a guess.
func ProcessRunning(name string) (bool, error) {
	code, err := runPgrep("pgrep", "-x", name)
	switch {
	case err == nil && code == 0:
		return true, nil
	case code == 1:
		return false, nil
	default:
		return false, fmt.Errorf("pgrep -x %s: %w", name, err)
	}
}

func runAppCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if message := strings.TrimSpace(string(output)); message != "" {
			return fmt.Errorf("%w: %s", err, message)
		}
		return err
	}
	return nil
}

// runPgrepCommand returns pgrep's exit code; err is set for any failure, including exit 1.
func runPgrepCommand(name string, args ...string) (int, error) {
	err := exec.Command(name, args...).Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}
	return -1, err
}
