package dockerdesktop

import (
	"fmt"

	"github.com/ningenai/dockguard/internal/config"
)

func Start(cfg config.Config) error {
	if cfg.DockerDesktopConfig.RequireCLIStartSupport {
		return fmt.Errorf("docker desktop start is not implemented yet")
	}

	return fmt.Errorf("docker desktop startup path is not configured")
}
