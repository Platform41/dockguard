package dockerdesktop

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/platform41/dockguard/internal/platform"
)

var (
	lookPath         = exec.LookPath
	runCmd           = runCommand
	runStatus        = runStatusCommand
	desktopProcesses = anyDesktopProcess
)

// desktopProcessPattern matches any process running from inside Docker Desktop's app bundle, such
// as "Docker Desktop" or com.docker.backend, wherever the app is installed. The docker CLI and its
// plugins live outside the bundle and don't match.
const desktopProcessPattern = `Docker\.app/Contents/`

// anyDesktopProcess reports whether any Docker Desktop process is running. pgrep exits 0 when it
// finds one and 1 when it finds none; anything else is an error.
func anyDesktopProcess() (bool, error) {
	_, code, err := platform.Run("pgrep", "-f", desktopProcessPattern)
	switch {
	case err != nil:
		return false, err
	case code == 0:
		return true, nil
	case code == 1:
		return false, nil
	default:
		return false, fmt.Errorf("pgrep exited %d", code)
	}
}

// StartOptions controls how Start behaves.
type StartOptions struct {
	RequireCLIStartSupport bool
	FailIfAlreadyRunning   bool
}

func Start(opts StartOptions) error {
	if _, err := lookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH: %w", err)
	}

	if opts.FailIfAlreadyRunning {
		running, err := IsRunning()
		if err != nil {
			return fmt.Errorf("check whether Docker Desktop is already running: %w", err)
		}
		if running {
			return fmt.Errorf("Docker Desktop is already running")
		}
	}

	if err := runCmd("docker", "desktop", "start"); err != nil {
		if opts.RequireCLIStartSupport {
			if isUnsupportedDesktopCLIError(err) {
				return fmt.Errorf("docker desktop start not supported; Docker Desktop 4.37+ required: %w", err)
			}
			return fmt.Errorf("start Docker Desktop with docker desktop start: %w", err)
		}
		return fmt.Errorf("Docker Desktop startup failed: %w", err)
	}

	return nil
}

func Stop() (bool, error) {
	if _, err := lookPath("docker"); err != nil {
		return false, fmt.Errorf("docker CLI not found in PATH: %w", err)
	}

	running, err := IsRunning()
	if err == nil && !running {
		return false, nil
	}

	if err := runCmd("docker", "desktop", "stop"); err != nil {
		if isDesktopAlreadyStoppedError(err) {
			return false, nil
		}
		return false, fmt.Errorf("stop Docker Desktop with docker desktop stop: %w", err)
	}

	return true, nil
}

// IsRunning reports whether Docker Desktop is running.
//
// When no Docker Desktop process exists, it's confirmed stopped without asking the CLI:
// `docker desktop status` waits about 16 seconds before reporting that Desktop isn't running. Any
// Desktop process, or a failed process check, falls through to the CLI, which stays the authority.
func IsRunning() (bool, error) {
	if found, err := desktopProcesses(); err == nil && !found {
		return false, nil
	}

	if _, err := lookPath("docker"); err != nil {
		return false, fmt.Errorf("docker CLI not found in PATH: %w", err)
	}

	output, err := runStatus("docker", "desktop", "status")
	if err != nil {
		if isUnsupportedDesktopCLIError(err) {
			return false, fmt.Errorf("docker desktop status not supported; Docker Desktop 4.37+ required: %w", err)
		}
		if isDesktopNotRunningOutput(output) {
			return false, nil
		}
		return false, err
	}

	status, ok := parseStatusLine(output)
	if !ok {
		return false, fmt.Errorf("unable to parse docker desktop status output")
	}

	return strings.EqualFold(status, "running"), nil
}

func runCommand(name string, args ...string) error {
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

func runStatusCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return string(output), fmt.Errorf("%w: %s", err, message)
		}
		return string(output), err
	}

	return string(output), nil
}

func isUnsupportedDesktopCLIError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unknown command") && strings.Contains(message, "desktop") {
		return true
	}
	if strings.Contains(message, "is not a docker command") && strings.Contains(message, "desktop") {
		return true
	}

	return false
}

func isDesktopNotRunningOutput(output string) bool {
	message := strings.ToLower(output)
	if strings.Contains(message, "could not retrieve status") && strings.Contains(message, "docker desktop") {
		return true
	}
	if strings.Contains(message, "is docker desktop running") {
		return true
	}

	return false
}

func isDesktopAlreadyStoppedError(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "docker desktop") && strings.Contains(message, "not running") {
		return true
	}

	return false
}

func parseStatusLine(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			label := strings.TrimSuffix(fields[0], ":")
			if strings.EqualFold(label, "Status") {
				return fields[1], true
			}
		}
	}

	return "", false
}
