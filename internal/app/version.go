package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/version"
)

const (
	latestCheckTimeout = 10 * time.Second
	updateTimeout      = 5 * time.Minute
)

var (
	currentVersion             = version.Current
	latestVersion              = version.Latest
	installPath                = version.InstallPath
	installUpdate              = version.Install
	installedVersion           = version.Installed
	executableFile             = resolvedExecutable
	versionTerminal            = stdioIsTerminal
	versionInput     io.Reader = os.Stdin
)

func runVersion() (int, error) {
	current := currentVersion()
	fmt.Fprintln(output.Out, current.String())

	ctx, cancel := context.WithTimeout(context.Background(), latestCheckTimeout)
	latest, err := latestVersion(ctx)
	cancel()
	if err != nil {
		printVersionCheckError(err)
		return 0, nil
	}
	if current.Version == "" {
		fmt.Fprintf(output.Out, "Latest release: %s.\n", latest.Version)
		return 0, nil
	}
	if !version.Newer(current.Version, latest.Version) {
		fmt.Fprintf(output.Out, "You're on the latest release (%s).\n", latest.Version)
		return 0, nil
	}

	fmt.Fprintf(output.Out, "A newer release is available: %s.\n", latest.Version)
	command := version.InstallCommand(latest.Version)
	if !versionTerminal() {
		fmt.Fprintf(output.Out, "To update, run: %s\n", command)
		return 0, nil
	}

	path, err := installPath()
	if err != nil {
		fmt.Fprintf(output.Out, "Can't update: %s.\n", versionErrorReason(err))
		return 1, nil
	}
	fmt.Fprintf(output.Out, "This runs: %s\n", command)
	fmt.Fprintf(output.Out, "and installs to %s.\n", path)
	printRunningCopyNote(path)
	fmt.Fprintf(output.Out, "Update to %s? [y/N] ", latest.Version)
	answer, _ := bufio.NewReader(versionInput).ReadString('\n')
	fmt.Fprintln(output.Out)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Fprintln(output.Out, "Not updated.")
		return 0, nil
	}

	ctx, cancel = context.WithTimeout(context.Background(), updateTimeout)
	err = installUpdate(ctx, latest.Version, output.Out)
	cancel()
	if err != nil {
		fmt.Fprintf(output.Out, "Update failed: %s.\n", versionErrorReason(err))
		return 1, nil
	}
	installed, err := installedVersion(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(output.Out, "go install finished, but %s doesn't exist; not updated.\n", path)
		return 1, nil
	case err != nil:
		fmt.Fprintf(output.Out, "go install finished, but %s can't be verified: %s.\n", path, versionErrorReason(err))
		return 1, nil
	case installed.Version != latest.Version:
		fmt.Fprintf(output.Out, "go install finished, but %s reports %s, not %s.\n", path, installed.Version, latest.Version)
		return 1, nil
	}
	fmt.Fprintf(output.Out, "Updated to %s.\n", latest.Version)
	printRunningCopyNote(path)
	return 0, nil
}

func printVersionCheckError(err error) {
	reason := versionErrorReason(err)
	if errors.Is(err, version.ErrGoNotInstalled) {
		reason = "the go command isn't installed"
	}
	fmt.Fprintf(output.Out, "Can't check for updates: %s.\n", reason)
}

func versionErrorReason(err error) string {
	return strings.TrimSuffix(strings.TrimSpace(err.Error()), ".")
}

func resolvedExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	return filepath.Clean(path), nil
}

func printRunningCopyNote(install string) {
	running, err := executableFile()
	if err != nil || samePath(running, install) {
		return
	}
	fmt.Fprintf(output.Out, "You're running %s; the update goes to %s and won't replace it.\n", running, install)
}

func samePath(a, b string) bool {
	resolve := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		return filepath.Clean(path)
	}
	return resolve(a) == resolve(b)
}
