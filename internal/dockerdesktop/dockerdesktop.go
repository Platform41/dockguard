package dockerdesktop

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/platform41/dockguard/internal/config"
)

var (
	lookPath = exec.LookPath
	runCmd   = runCommand
	runQuery = runQueryCommand
)

func Start(cfg config.Config) error {
	if _, err := lookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH: %w", err)
	}

	if cfg.DockerDesktopConfig.FailIfAlreadyRunning {
		running, err := IsRunning()
		if err != nil {
			return fmt.Errorf("check whether Docker Desktop is already running: %w", err)
		}
		if running {
			return fmt.Errorf("Docker Desktop is already running")
		}
	}

	if err := runCmd("docker", "desktop", "start"); err != nil {
		if cfg.DockerDesktopConfig.RequireCLIStartSupport {
			return fmt.Errorf("start Docker Desktop with docker desktop start: %w", err)
		}
		return fmt.Errorf("Docker Desktop startup failed: %w", err)
	}

	return nil
}

func IsRunning() (bool, error) {
	if _, err := lookPath("pgrep"); err != nil {
		return false, fmt.Errorf("pgrep not found in PATH: %w", err)
	}

	if err := runQuery("pgrep", "-x", "Docker"); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}

	return true, nil
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

func runQueryCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}
