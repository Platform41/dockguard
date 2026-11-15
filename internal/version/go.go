package version

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/platform"
)

var (
	ErrGoNotInstalled  = errors.New("go command is not installed")
	ErrModuleIsolation = errors.New("couldn't run outside a Go module")

	runDetachedCommand = platform.RunDetachedCommand
	readInstalledInfo  = buildinfo.ReadFile
	mkdirTemp          = os.MkdirTemp
	removeAll          = os.RemoveAll
	environ            = os.Environ
)

// Latest asks the Go toolchain for the newest module release.
func Latest(ctx context.Context) (Info, error) {
	output, err := goOutput(ctx, "env", "GOMOD")
	if err != nil {
		return Info{}, err
	}
	if strings.TrimSpace(output) != "/dev/null" {
		return Info{}, ErrModuleIsolation
	}

	output, err = goOutput(ctx, "list", "-m", "-json", modulePath+"@latest")
	if err != nil {
		return Info{}, err
	}
	var module struct {
		Version string `json:"Version"`
	}
	if err := json.Unmarshal([]byte(output), &module); err != nil {
		return Info{}, fmt.Errorf("couldn't read the latest release: %w", err)
	}
	if _, ok := parseSemanticVersion(module.Version); !ok {
		return Info{}, fmt.Errorf("couldn't read the latest release version %q", module.Version)
	}
	return Info{Version: module.Version}, nil
}

// InstallPath returns where go install will place DockGuard.
func InstallPath() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := goOutput(ctx, "env", "GOBIN", "GOPATH")
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	if len(lines) != 2 {
		return "", fmt.Errorf("couldn't read GOBIN and GOPATH")
	}
	bin := strings.TrimSpace(lines[0])
	if bin == "" {
		paths := filepath.SplitList(strings.TrimSpace(lines[1]))
		if len(paths) == 0 || paths[0] == "" {
			return "", fmt.Errorf("GOBIN and GOPATH are empty")
		}
		bin = filepath.Join(paths[0], "bin")
	}
	return filepath.Join(bin, "dockguard"), nil
}

// Install runs go install for one pinned release and streams its output.
func Install(ctx context.Context, release string, stream io.Writer) error {
	_, _, err := runGo(ctx, stream, "install", commandPath+"@"+release)
	return err
}

// Installed reads build information from an installed DockGuard file.
func Installed(path string) (Info, error) {
	build, err := readInstalledInfo(path)
	if err != nil {
		return Info{}, err
	}
	return Info{Version: build.Main.Version, GoVersion: build.GoVersion}, nil
}

func goOutput(ctx context.Context, args ...string) (string, error) {
	output, code, err := runGo(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		if reason := strings.TrimSpace(output); reason != "" {
			return "", errors.New(reason)
		}
		return "", fmt.Errorf("go %s exited %d", args[0], code)
	}
	return output, nil
}

func runGo(ctx context.Context, stream io.Writer, args ...string) (string, int, error) {
	dir, err := mkdirTemp("", "dockguard-version-*")
	if err != nil {
		return "", -1, err
	}
	defer removeAll(dir)

	cmd := platform.NewDetachedCommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = replaceEnv(environ(), "GOWORK", "off")
	output, code, err := runDetachedCommand(ctx, cmd, stream)
	if errors.Is(err, exec.ErrNotFound) {
		return output, code, fmt.Errorf("%w: %v", ErrGoNotInstalled, err)
	}
	if err != nil {
		return output, code, err
	}
	if code != 0 && stream != nil {
		if reason := strings.TrimSpace(output); reason != "" {
			return output, code, fmt.Errorf("go %s exited %d: %s", args[0], code, reason)
		}
		return output, code, fmt.Errorf("go %s exited %d", args[0], code)
	}
	return output, code, nil
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}
