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
	command, configPath, err := parseArgs(args)
	if err != nil {
		return 1, err
	}

	if command == "" {
		output.PrintUsage()
		return 1, nil
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return 1, fmt.Errorf("load config: %w", err)
	}

	switch command {
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

func parseArgs(args []string) (command string, configPath string, err error) {
	if len(args) == 0 {
		return "", "", nil
	}

	command = args[0]
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) {
				return "", "", errors.New("--config requires a path")
			}
			configPath = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("unknown argument %q", args[i])
		}
	}

	return command, configPath, nil
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
