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
)

func Start(cfg config.Config) error {
	if _, err := lookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found in PATH: %w", err)
	}

	if err := runCmd("docker", "desktop", "start"); err != nil {
		if cfg.DockerDesktopConfig.RequireCLIStartSupport {
			return fmt.Errorf("start Docker Desktop with docker desktop start: %w", err)
		}
		return fmt.Errorf("Docker Desktop startup failed: %w", err)
	}

	return nil
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
