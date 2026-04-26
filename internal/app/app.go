package app

import (
	"errors"
	"fmt"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/dockerdesktop"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/platform"
)

type options struct {
	Command    string
	ConfigPath string
	Eject      bool
}

func Run(args []string) (int, error) {
	opts, err := parseArgs(args)
	if err != nil {
		return 1, err
	}

	if opts.Command == "" {
		output.PrintUsage()
		return 1, nil
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return 1, fmt.Errorf("load config: %w", err)
	}

	switch opts.Command {
	case "status":
		return runStatus(cfg)
	case "check":
		return runCheck(cfg)
	case "start":
		return runStart(cfg)
	case "stop":
		return runStop(cfg, opts.Eject)
	case "help", "--help", "-h":
		output.PrintUsage()
		return 0, nil
	default:
		return 1, fmt.Errorf("unknown command %q", args[0])
	}
}

func parseArgs(args []string) (options, error) {
	if len(args) == 0 {
		return options{}, nil
	}

	opts := options{Command: args[0]}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--config":
			if i+1 >= len(args) {
				return options{}, errors.New("--config requires a path")
			}
			opts.ConfigPath = args[i+1]
			i++
		case "--eject":
			opts.Eject = true
		default:
			return options{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}

	if opts.Eject && opts.Command != "stop" {
		return options{}, errors.New("--eject is only supported with the stop command")
	}

	return opts, nil
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

func runStop(cfg config.Config, eject bool) (int, error) {
	stopRequested, err := dockerdesktop.Stop()
	if err != nil {
		return 1, err
	}
	if stopRequested {
		output.PrintStopped()
	} else {
		output.PrintAlreadyStopped()
	}

	if eject {
		if err := platform.EjectVolume(cfg.ExternalMountPath); err != nil {
			return 1, err
		}
		output.PrintEjected(cfg.ExternalMountPath)
	}

	return 0, nil
}
