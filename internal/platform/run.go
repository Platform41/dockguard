package platform

import (
	"errors"
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
