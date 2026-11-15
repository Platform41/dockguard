package platform

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
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
	return RunDetachedCommand(nil, exec.Command(name, args...), nil)
}

// RunDetachedContext is RunDetached with cancellation. The process group is killed so a readiness
// script's descendants cannot survive its deadline or keep the caller waiting.
func RunDetachedContext(ctx context.Context, name string, args ...string) (output string, code int, err error) {
	return RunDetachedCommand(ctx, NewDetachedCommandContext(ctx, name, args...), nil)
}

// NewDetachedCommandContext configures a command whose process group is killed on cancellation.
func NewDetachedCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
}

// RunDetachedCommand runs a configured detached command and optionally streams its combined output.
func RunDetachedCommand(ctx context.Context, cmd *exec.Cmd, stream io.Writer) (output string, code int, err error) {
	file, err := os.CreateTemp("", "dockguard-run-*")
	if err != nil {
		return "", -1, err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	var writer io.Writer = file
	if stream != nil {
		writer = io.MultiWriter(file, stream)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	runErr := cmd.Run()

	out, _ := os.ReadFile(file.Name())
	if runErr == nil {
		return string(out), 0, nil
	}
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return string(out), -1, ctxErr
		}
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, runErr
}
