package platform

import (
	"errors"
	"os"
	"os/exec"
)

// Run runs a command and returns its combined output and exit code. err is set only when the
// command couldn't be run at all (for example, the binary is missing); a non-zero exit is reported
// through the code, not err, so callers can tell "exited 3" from "didn't run".
func Run(name string, args ...string) (output string, code int, err error) {
	out, runErr := exec.Command(name, args...).CombinedOutput()
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, runErr
}

// RunDetached is Run for commands that start a long-lived child, such as pg_ctl start. Output goes
// to a temporary file instead of a pipe: a child that inherits the output would hold a pipe open
// and make the caller wait for it to exit, but a file can't. It returns once the command exits.
func RunDetached(name string, args ...string) (output string, code int, err error) {
	file, err := os.CreateTemp("", "dockguard-run-*")
	if err != nil {
		return "", -1, err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	cmd := exec.Command(name, args...)
	cmd.Stdout = file
	cmd.Stderr = file
	runErr := cmd.Run()

	out, _ := os.ReadFile(file.Name())
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, runErr
}
