package app

import (
	"errors"
	"fmt"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/dockerdesktop"
	"github.com/platform41/dockguard/internal/output"
)

func Run(args []string) (int, error) {
	if len(args) == 0 {
		output.PrintUsage()
		return 1, nil
	}

	cfg, err := config.LoadDefault()
	if err != nil {
		return 1, fmt.Errorf("load config: %w", err)
	}

	switch args[0] {
	case "status":
		return runStatus(cfg)
	case "check":
		return runCheck(cfg)
	case "start":
		return runStart(cfg)
	case "help", "--help", "-h":
		output.PrintUsage()
		return 0, nil
	default:
		return 1, fmt.Errorf("unknown command %q", args[0])
	}
}

func runStatus(cfg config.Config) (int, error) {
	result := checks.BuildStatus(cfg)
	output.PrintStatus(result)
	if result.Ready {
		return 0, nil
	}
	return 1, nil
}

func runCheck(cfg config.Config) (int, error) {
	result := checks.RunPreflight(cfg)
	output.PrintCheckResult(result)
	if result.OK {
		return 0, nil
	}
	return 1, errors.New("preflight checks failed")
}

func runStart(cfg config.Config) (int, error) {
	result := checks.RunPreflight(cfg)
	output.PrintCheckResult(result)
	if !result.OK {
		return 1, errors.New("refusing to start Docker Desktop because preflight checks failed")
	}

	if err := dockerdesktop.Start(cfg); err != nil {
		return 1, err
	}

	output.PrintStarted()
	return 0, nil
}
