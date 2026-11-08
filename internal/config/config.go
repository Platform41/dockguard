// Package config loads DockGuard's config file: the record of which applications DockGuard
// manages on the external drive (docs/specs/multi-target.md §3).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/platform41/dockguard/internal/pathutil"
)

// DefaultConfigPath is the file looked for in the current folder (v0.1 behaviour).
const DefaultConfigPath = "dockguard.yaml"

// UserConfigPath is the user-level config, relative to the home folder (D-015).
const UserConfigPath = ".config/dockguard/dockguard.yaml"

const defaultMinimumFreeSpaceGB = 50

// Kinds.
const (
	KindDockerDesktop = "docker-desktop"
	KindApp           = "app"
	KindPostgres      = "postgres"
	KindContainer     = "container"
)

// plannedKinds are in the spec but not built yet.
var plannedKinds = map[string]string{
	KindPostgres:  "v0.3.0",
	KindContainer: "v0.4.0",
}

var targetNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Config is a loaded config file.
type Config struct {
	// Path is the file the config was read from.
	Path string
	// Legacy is true when the file used the v0.1 format.
	Legacy bool
	// Drive is the external volume every target depends on.
	Drive string
	// MinimumFreeSpaceGB is the default free-space threshold for every target.
	MinimumFreeSpaceGB int
	// Targets are in file order.
	Targets []Target
}

// Target is one managed application.
type Target struct {
	Name                 string
	Kind                 string
	MinimumFreeSpaceGB   int
	FailIfAlreadyRunning bool
	DockerDesktop        *DockerDesktop
	App                  *App
}

// DockerDesktop holds the keys of a docker-desktop target.
type DockerDesktop struct {
	StoragePath            string
	SettingsPath           string
	RequireCLIStartSupport bool
}

// App holds the keys of an app target.
type App struct {
	// Name is what `open -a` takes, for example "Xcode".
	Name string
	// Process is the process name pgrep looks for; it defaults to Name.
	Process string
	// SymlinkPaths are paths on the internal disk that link to the drive.
	SymlinkPaths []string
}

// Find returns the target with the given name.
func (c Config) Find(name string) (Target, bool) {
	for _, t := range c.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

// Names returns the target names in file order.
func (c Config) Names() []string {
	names := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		names = append(names, t.Name)
	}
	return names
}

// ErrNotFound is returned when no config file exists in any lookup location.
var ErrNotFound = errors.New("config file not found")

// Resolve returns the config file to read: the explicit path if given, else ./dockguard.yaml if
// present, else ~/.config/dockguard/dockguard.yaml if present.
func Resolve(explicit string) (string, error) {
	if explicit != "" {
		path := expandPath(explicit)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("%w: %s", ErrNotFound, path)
			}
			return "", fmt.Errorf("stat config %q: %w", path, err)
		}
		return path, nil
	}

	candidates := []string{DefaultConfigPath}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, UserConfigPath))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat config %q: %w", candidate, err)
		}
	}

	return "", fmt.Errorf("%w; tried %s", ErrNotFound, strings.Join(candidates, ", "))
}

// Load finds and reads the config file.
func Load(explicit string) (Config, error) {
	path, err := Resolve(explicit)
	if err != nil {
		return Config{}, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	cfg, err := Parse(content)
	if err != nil {
		return Config{}, fmt.Errorf("config %q: %w", path, err)
	}
	cfg.Path = path
	return cfg, nil
}

var newFormatKeys = map[string]bool{"drive": true, "targets": true}
var legacyKeys = map[string]bool{"external_mount_path": true, "docker_storage_path": true, "docker_desktop": true}

// Parse reads config content in either the current or the v0.1 format.
func Parse(content []byte) (Config, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return Config{}, fmt.Errorf("parse YAML: %w", err)
	}
	if root.Kind == 0 || len(root.Content) == 0 {
		return Config{}, errors.New("config is empty")
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return Config{}, errors.New("config must be a mapping of keys")
	}

	var newSeen, legacySeen []string
	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i].Value
		switch {
		case key == "minimum_free_space_gb":
		case newFormatKeys[key]:
			newSeen = append(newSeen, key)
		case legacyKeys[key]:
			legacySeen = append(legacySeen, key)
		default:
			return Config{}, fmt.Errorf("line %d: unknown key %q", doc.Content[i].Line, key)
		}
	}

	if len(newSeen) > 0 && len(legacySeen) > 0 {
		return Config{}, fmt.Errorf(
			"mixes the current format (%s) with the v0.1 format (%s); use one",
			strings.Join(newSeen, ", "), strings.Join(legacySeen, ", "))
	}

	if len(newSeen) == 0 {
		return parseLegacy(doc)
	}
	return parseCurrent(doc)
}

type currentFile struct {
	Drive              string    `yaml:"drive"`
	MinimumFreeSpaceGB *int      `yaml:"minimum_free_space_gb"`
	Targets            yaml.Node `yaml:"targets"`
}

func parseCurrent(doc *yaml.Node) (Config, error) {
	var file currentFile
	if err := doc.Decode(&file); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Drive:              cleanPath(file.Drive),
		MinimumFreeSpaceGB: defaultMinimumFreeSpaceGB,
	}
	if cfg.Drive == "" {
		return Config{}, errors.New("missing required key \"drive\"")
	}
	if file.MinimumFreeSpaceGB != nil {
		if *file.MinimumFreeSpaceGB <= 0 {
			return Config{}, errors.New("minimum_free_space_gb must be greater than zero")
		}
		cfg.MinimumFreeSpaceGB = *file.MinimumFreeSpaceGB
	}

	targets := &file.Targets
	if targets.Kind == 0 {
		return Config{}, errors.New("missing required key \"targets\"")
	}
	if targets.Kind != yaml.MappingNode || len(targets.Content) == 0 {
		return Config{}, errors.New("\"targets\" must list at least one target")
	}

	seen := map[string]bool{}
	for i := 0; i < len(targets.Content); i += 2 {
		name := targets.Content[i].Value
		if !targetNamePattern.MatchString(name) {
			return Config{}, fmt.Errorf("target %q: names use lowercase letters, digits and '-'", name)
		}
		if seen[name] {
			return Config{}, fmt.Errorf("target %q is listed twice", name)
		}
		seen[name] = true

		target, err := parseTarget(name, targets.Content[i+1], cfg.MinimumFreeSpaceGB)
		if err != nil {
			return Config{}, fmt.Errorf("target %q: %w", name, err)
		}
		cfg.Targets = append(cfg.Targets, target)
	}

	return cfg, nil
}

type dockerDesktopKeys struct {
	Kind                   string `yaml:"kind"`
	StoragePath            string `yaml:"storage_path"`
	SettingsPath           string `yaml:"settings_path"`
	RequireCLIStartSupport *bool  `yaml:"require_cli_start_support"`
	FailIfAlreadyRunning   *bool  `yaml:"fail_if_already_running"`
	MinimumFreeSpaceGB     *int   `yaml:"minimum_free_space_gb"`
}

var allowedKeys = map[string][]string{
	KindDockerDesktop: {"kind", "storage_path", "settings_path", "require_cli_start_support", "fail_if_already_running", "minimum_free_space_gb"},
	KindApp:           {"kind", "app", "process", "symlink_paths", "fail_if_already_running"},
}

func parseTarget(name string, node *yaml.Node, defaultFreeGB int) (Target, error) {
	if node.Kind != yaml.MappingNode {
		return Target{}, errors.New("must be a mapping of keys")
	}

	keys := map[string]int{}
	kind := ""
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		keys[key] = node.Content[i].Line
		if key == "kind" {
			kind = node.Content[i+1].Value
		}
	}

	if kind == "" {
		return Target{}, errors.New("missing required key \"kind\"")
	}
	if release, planned := plannedKinds[kind]; planned {
		return Target{}, fmt.Errorf("kind %q is planned for %s and not available in this version", kind, release)
	}
	allowed, known := allowedKeys[kind]
	if !known {
		return Target{}, fmt.Errorf("unknown kind %q", kind)
	}
	if err := checkKeys(keys, allowed); err != nil {
		return Target{}, err
	}

	switch kind {
	case KindApp:
		return parseApp(name, node)
	default:
		return parseDockerDesktop(name, node, defaultFreeGB)
	}
}

func checkKeys(keys map[string]int, allowed []string) error {
	ok := map[string]bool{}
	for _, k := range allowed {
		ok[k] = true
	}
	var unknown []string
	for k := range keys {
		if !ok[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("line %d: unknown key %q", keys[unknown[0]], unknown[0])
}

func parseDockerDesktop(name string, node *yaml.Node, defaultFreeGB int) (Target, error) {
	var keys dockerDesktopKeys
	if err := node.Decode(&keys); err != nil {
		return Target{}, err
	}
	if keys.StoragePath == "" {
		return Target{}, errors.New("missing required key \"storage_path\"")
	}

	target := Target{
		Name:                 name,
		Kind:                 KindDockerDesktop,
		MinimumFreeSpaceGB:   defaultFreeGB,
		FailIfAlreadyRunning: boolOr(keys.FailIfAlreadyRunning, true),
		DockerDesktop: &DockerDesktop{
			StoragePath:            cleanPath(keys.StoragePath),
			SettingsPath:           cleanPath(keys.SettingsPath),
			RequireCLIStartSupport: boolOr(keys.RequireCLIStartSupport, true),
		},
	}
	if keys.MinimumFreeSpaceGB != nil {
		if *keys.MinimumFreeSpaceGB <= 0 {
			return Target{}, errors.New("minimum_free_space_gb must be greater than zero")
		}
		target.MinimumFreeSpaceGB = *keys.MinimumFreeSpaceGB
	}
	if target.DockerDesktop.SettingsPath == "" {
		settings, err := DetectSettingsPath()
		if err != nil {
			return Target{}, err
		}
		target.DockerDesktop.SettingsPath = settings
	}
	return target, nil
}

type appKeys struct {
	Kind                 string   `yaml:"kind"`
	App                  string   `yaml:"app"`
	Process              string   `yaml:"process"`
	SymlinkPaths         []string `yaml:"symlink_paths"`
	FailIfAlreadyRunning *bool    `yaml:"fail_if_already_running"`
}

func parseApp(name string, node *yaml.Node) (Target, error) {
	var keys appKeys
	if err := node.Decode(&keys); err != nil {
		return Target{}, err
	}
	appName := strings.TrimSpace(keys.App)
	if appName == "" {
		return Target{}, errors.New("missing required key \"app\"")
	}

	app := &App{Name: appName, Process: stringOr(strings.TrimSpace(keys.Process), appName)}
	for _, raw := range keys.SymlinkPaths {
		path := cleanPath(raw)
		if path == "" {
			return Target{}, errors.New("symlink_paths has an empty entry")
		}
		if err := rejectCoreSimulator(path); err != nil {
			return Target{}, err
		}
		app.SymlinkPaths = append(app.SymlinkPaths, path)
	}

	return Target{
		Name:                 name,
		Kind:                 KindApp,
		FailIfAlreadyRunning: boolOr(keys.FailIfAlreadyRunning, true),
		App:                  app,
	}, nil
}

// rejectCoreSimulator enforces D-013: simulator devices break on an external drive, so DockGuard
// refuses the Devices folder, anything inside it, and any folder that contains it.
func rejectCoreSimulator(path string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	devices := filepath.Join(home, "Library", "Developer", "CoreSimulator", "Devices")
	clean := filepath.Clean(path)

	// Compare both the spellings and the real paths, so an alias such as a symlink to
	// CoreSimulator can't slip past the rule.
	realPath, realDevices := pathutil.Resolve(clean), pathutil.Resolve(devices)
	if pathutil.Within(clean, devices) || pathutil.Within(devices, clean) ||
		pathutil.Within(realPath, realDevices) || pathutil.Within(realDevices, realPath) {
		return fmt.Errorf("symlink_paths can't include %s: moving simulator devices to an external drive breaks `simctl create`, with no workaround", clean)
	}
	return nil
}

// v0.1 format: one Docker Desktop setup, mapped to a single target named "docker".

const (
	legacyMountPath   = "/Volumes/DockerSSD"
	legacyStoragePath = "/Volumes/DockerSSD/DockerDesktop"
)

type legacyFile struct {
	ExternalMountPath  string `yaml:"external_mount_path"`
	DockerStoragePath  string `yaml:"docker_storage_path"`
	MinimumFreeSpaceGB *int   `yaml:"minimum_free_space_gb"`
	DockerDesktop      struct {
		SettingsPath           string `yaml:"settings_path"`
		RequireCLIStartSupport *bool  `yaml:"require_cli_start_support"`
		FailIfAlreadyRunning   *bool  `yaml:"fail_if_already_running"`
	} `yaml:"docker_desktop"`
}

func parseLegacy(doc *yaml.Node) (Config, error) {
	var buf bytes.Buffer
	if err := yaml.NewEncoder(&buf).Encode(doc); err != nil {
		return Config{}, err
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)

	var file legacyFile
	if err := dec.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("v0.1 format: %w", err)
	}

	cfg := Config{
		Legacy:             true,
		Drive:              stringOr(cleanPath(file.ExternalMountPath), legacyMountPath),
		MinimumFreeSpaceGB: defaultMinimumFreeSpaceGB,
	}
	if file.MinimumFreeSpaceGB != nil {
		cfg.MinimumFreeSpaceGB = *file.MinimumFreeSpaceGB
	}

	settings := cleanPath(file.DockerDesktop.SettingsPath)
	if settings == "" {
		detected, err := DetectSettingsPath()
		if err != nil {
			return Config{}, err
		}
		settings = detected
	}

	cfg.Targets = []Target{{
		Name:                 "docker",
		Kind:                 KindDockerDesktop,
		MinimumFreeSpaceGB:   cfg.MinimumFreeSpaceGB,
		FailIfAlreadyRunning: boolOr(file.DockerDesktop.FailIfAlreadyRunning, true),
		DockerDesktop: &DockerDesktop{
			StoragePath:            stringOr(cleanPath(file.DockerStoragePath), legacyStoragePath),
			SettingsPath:           settings,
			RequireCLIStartSupport: boolOr(file.DockerDesktop.RequireCLIStartSupport, true),
		},
	}}
	return cfg, nil
}

// DetectSettingsPath returns the first Docker Desktop settings file that exists, or the newest
// layout's path when none does.
func DetectSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return detectSettingsPath(home)
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

func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func stringOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func cleanPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return expandPath(path)
}

func expandPath(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
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
