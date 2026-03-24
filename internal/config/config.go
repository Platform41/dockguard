package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const DefaultConfigPath = "dockguard.yaml"

type Config struct {
	ExternalMountPath   string
	DockerStoragePath   string
	MinimumFreeSpaceGB  int
	DockerDesktopConfig DockerDesktopConfig
}

type DockerDesktopConfig struct {
	SettingsPath           string
	RequireCLIStartSupport bool
	FailIfAlreadyRunning   bool
}

func Load(path string) (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}

	if path == "" {
		path = DefaultConfigPath
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("stat config %q: %w", path, err)
	}

	loaded, err := parseFile(path, cfg)
	if err != nil {
		return Config{}, err
	}

	return loaded, nil
}

func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve home directory: %w", err)
	}

	settingsPath, err := detectSettingsPath(home)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ExternalMountPath:  "/Volumes/DockerSSD",
		DockerStoragePath:  "/Volumes/DockerSSD/DockerDesktop",
		MinimumFreeSpaceGB: 50,
		DockerDesktopConfig: DockerDesktopConfig{
			SettingsPath:           settingsPath,
			RequireCLIStartSupport: true,
			FailIfAlreadyRunning:   true,
		},
	}, nil
}

func parseFile(path string, cfg Config) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	lineNo := 0

	for scanner.Scan() {
		lineNo++

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") {
			section = strings.TrimSuffix(line, ":")
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("parse config %q line %d: expected key: value", path, lineNo)
		}

		key = strings.TrimSpace(key)
		value = cleanValue(value)

		switch section {
		case "":
			if err := assignTopLevel(&cfg, key, value); err != nil {
				return Config{}, fmt.Errorf("parse config %q line %d: %w", path, lineNo, err)
			}
		case "docker_desktop":
			if err := assignDockerDesktop(&cfg.DockerDesktopConfig, key, value); err != nil {
				return Config{}, fmt.Errorf("parse config %q line %d: %w", path, lineNo, err)
			}
		default:
			return Config{}, fmt.Errorf("parse config %q line %d: unsupported section %q", path, lineNo, section)
		}
	}

	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg.ExternalMountPath = expandPath(cfg.ExternalMountPath)
	cfg.DockerStoragePath = expandPath(cfg.DockerStoragePath)
	cfg.DockerDesktopConfig.SettingsPath = expandPath(cfg.DockerDesktopConfig.SettingsPath)

	return cfg, nil
}

func assignTopLevel(cfg *Config, key, value string) error {
	switch key {
	case "external_mount_path":
		cfg.ExternalMountPath = expandPath(value)
	case "docker_storage_path":
		cfg.DockerStoragePath = expandPath(value)
	case "minimum_free_space_gb":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("minimum_free_space_gb must be an integer")
		}
		cfg.MinimumFreeSpaceGB = n
	default:
		return fmt.Errorf("unsupported key %q", key)
	}

	return nil
}

func assignDockerDesktop(cfg *DockerDesktopConfig, key, value string) error {
	switch key {
	case "settings_path":
		cfg.SettingsPath = expandPath(value)
	case "require_cli_start_support":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("require_cli_start_support must be true or false")
		}
		cfg.RequireCLIStartSupport = b
	case "fail_if_already_running":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("fail_if_already_running must be true or false")
		}
		cfg.FailIfAlreadyRunning = b
	default:
		return fmt.Errorf("unsupported docker_desktop key %q", key)
	}

	return nil
}

func detectSettingsPath(home string) (string, error) {
	candidates := []string{
		filepath.Join(home, "Library", "Application Support", "Docker", "settings-store.json"),
		filepath.Join(home, "Library", "Group Containers", "group.com.docker", "settings-store.json"),
		filepath.Join(home, "Library", "Application Support", "Docker", "settings.json"),
		filepath.Join(home, "Library", "Group Containers", "group.com.docker", "settings.json"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat Docker settings candidate %q: %w", candidate, err)
		}
	}

	return candidates[0], nil
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)

	return value
}

func expandPath(path string) string {
	if path == "" || path == "~" {
		if path == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				return home
			}
		}
		return path
	}

	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}

	return path
}
