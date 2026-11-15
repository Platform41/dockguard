package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/target"
	"github.com/platform41/dockguard/internal/ui"
	"github.com/platform41/dockguard/internal/version"
)

type versionHarness struct {
	out            bytes.Buffer
	latest         version.Info
	latestErr      error
	terminal       bool
	input          string
	installPath    string
	pathCalls      int
	installCalls   int
	installErr     error
	installOutput  string
	installed      version.Info
	installedErr   error
	verifyCalls    int
	checkCalls     int
	latestTimeout  time.Duration
	installTimeout time.Duration
}

func setupVersion(t *testing.T, current, latest string) *versionHarness {
	t.Helper()
	h := &versionHarness{
		latest:      version.Info{Version: latest},
		terminal:    true,
		installPath: "/home/me/go/bin/dockguard",
		installed:   version.Info{Version: latest},
	}
	original := struct {
		current    func() version.Info
		latest     func(context.Context) (version.Info, error)
		path       func() (string, error)
		install    func(context.Context, string, io.Writer) error
		installed  func(string) (version.Info, error)
		executable func() (string, error)
		terminal   func() bool
		input      io.Reader
		out        io.Writer
	}{currentVersion, latestVersion, installPath, installUpdate, installedVersion, executableFile, versionTerminal, versionInput, output.Out}
	t.Cleanup(func() {
		currentVersion, latestVersion, installPath, installUpdate = original.current, original.latest, original.path, original.install
		installedVersion, executableFile, versionTerminal = original.installed, original.executable, original.terminal
		versionInput, output.Out = original.input, original.out
	})

	currentVersion = func() version.Info {
		if current == "" {
			return version.Info{}
		}
		return version.Info{Version: current, GoVersion: "go1.26.1", Platform: "darwin/arm64"}
	}
	latestVersion = func(ctx context.Context) (version.Info, error) {
		h.checkCalls++
		if deadline, ok := ctx.Deadline(); ok {
			h.latestTimeout = time.Until(deadline).Round(time.Second)
		}
		return h.latest, h.latestErr
	}
	installPath = func() (string, error) { h.pathCalls++; return h.installPath, nil }
	installUpdate = func(ctx context.Context, release string, stream io.Writer) error {
		h.installCalls++
		if release != h.latest.Version {
			t.Fatalf("install release = %q, want %q", release, h.latest.Version)
		}
		if deadline, ok := ctx.Deadline(); ok {
			h.installTimeout = time.Until(deadline).Round(time.Second)
		}
		_, _ = io.WriteString(stream, h.installOutput)
		return h.installErr
	}
	installedVersion = func(path string) (version.Info, error) {
		h.verifyCalls++
		if path != h.installPath {
			t.Fatalf("verified path = %q, want %q", path, h.installPath)
		}
		return h.installed, h.installedErr
	}
	executableFile = func() (string, error) { return h.installPath, nil }
	versionTerminal = func() bool { return h.terminal }
	versionInput = strings.NewReader("")
	output.Out = &h.out
	return h
}

func setVersionInput(h *versionHarness, input string) {
	h.input = input
	versionInput = strings.NewReader(input)
}

// AS-88: an up-to-date release prints both version lines and never prompts.
func TestVersionLatestAS88(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "v0.6.0")
	code, err := Run([]string{"version"})
	want := "DockGuard v0.6.0 (go1.26.1, darwin/arm64)\nYou're on the latest release (v0.6.0).\n"
	if code != 0 || err != nil || h.out.String() != want || h.installCalls != 0 {
		t.Fatalf("version = %d, %v, output %q", code, err, h.out.String())
	}
	if h.latestTimeout < 9*time.Second || h.latestTimeout > 10*time.Second {
		t.Fatalf("latest timeout = %v", h.latestTimeout)
	}
}

// AS-90: yes installs the pinned version and verifies the installed file.
func TestVersionUpdateAS90(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "v0.7.0")
	setVersionInput(h, "y\n")
	code, err := Run([]string{"version"})
	if code != 0 || err != nil || h.installCalls != 1 || h.verifyCalls != 1 {
		t.Fatalf("version = %d, %v; install %d, verify %d", code, err, h.installCalls, h.verifyCalls)
	}
	for _, want := range []string{
		"A newer release is available: v0.7.0.",
		"This runs: go install github.com/platform41/dockguard/cmd/dockguard@v0.7.0",
		"and installs to /home/me/go/bin/dockguard.",
		"Update to v0.7.0? [y/N]",
		"Updated to v0.7.0.",
	} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, h.out.String())
		}
	}
	if h.installTimeout < 4*time.Minute+59*time.Second || h.installTimeout > 5*time.Minute {
		t.Fatalf("install timeout = %v", h.installTimeout)
	}
}

// AS-91: Enter, negative or unknown answers, and EOF all default to no update.
func TestVersionUpdateDefaultsNoAS91(t *testing.T) {
	for _, input := range []string{"\n", "n\n", "other text\n", "yes please\n", ""} {
		h := setupVersion(t, "v0.6.0", "v0.7.0")
		setVersionInput(h, input)
		code, err := Run([]string{"version"})
		if code != 0 || err != nil || h.installCalls != 0 || !strings.Contains(h.out.String(), "Not updated.") {
			t.Fatalf("input %q: version = %d, %v; install %d, output:\n%s", input, code, err, h.installCalls, h.out.String())
		}
	}
}

// AS-92: non-terminals print the update command without resolving a path or prompting.
func TestVersionNonTerminalAS92(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "v0.7.0")
	h.terminal = false
	code, err := Run([]string{"version"})
	want := "To update, run: go install github.com/platform41/dockguard/cmd/dockguard@v0.7.0"
	if code != 0 || err != nil || h.pathCalls != 0 || h.installCalls != 0 ||
		!strings.Contains(h.out.String(), want) || strings.Contains(h.out.String(), "[y/N]") {
		t.Fatalf("version = %d, %v; paths %d, install %d, output:\n%s", code, err, h.pathCalls, h.installCalls, h.out.String())
	}
}

// AS-93: a missing go command makes only the informational check unavailable.
func TestVersionGoMissingAS93(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "")
	h.latestErr = version.ErrGoNotInstalled
	code, err := Run([]string{"version"})
	if code != 0 || err != nil || !strings.Contains(h.out.String(), "Can't check for updates: the go command isn't installed.") {
		t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
	}
}

// AS-94: release-check errors and timeouts do not fail the version command.
func TestVersionCheckFailureAS94(t *testing.T) {
	for _, reason := range []error{errors.New("module lookup disabled by GOPROXY=off"), context.DeadlineExceeded} {
		h := setupVersion(t, "v0.6.0", "")
		h.latestErr = reason
		code, err := Run([]string{"version"})
		if code != 0 || err != nil || !strings.Contains(h.out.String(), "Can't check for updates: "+reason.Error()) {
			t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
		}
	}
}

// AS-95: a different running copy is named before the prompt and again after a successful update.
func TestVersionRunningCopyNoteAS95(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "v0.7.0")
	setVersionInput(h, "yes\n")
	executableFile = func() (string, error) { return "/usr/local/bin/dockguard", nil }
	code, err := Run([]string{"version"})
	note := "You're running /usr/local/bin/dockguard; the update goes to /home/me/go/bin/dockguard and won't replace it."
	first := strings.Index(h.out.String(), note)
	last := strings.LastIndex(h.out.String(), note)
	prompt := strings.Index(h.out.String(), "Update to v0.7.0?")
	updated := strings.Index(h.out.String(), "Updated to v0.7.0.")
	if code != 0 || err != nil || first < 0 || first > prompt || last < updated || first == last {
		t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
	}
}

// AS-96: a failed install shows its streamed output, skips verification, and exits 1.
func TestVersionInstallFailureAS96(t *testing.T) {
	h := setupVersion(t, "v0.6.0", "v0.7.0")
	setVersionInput(h, "y\n")
	h.installOutput = "module lookup failed\n"
	h.installErr = errors.New("exit 1")
	code, err := Run([]string{"version"})
	if code != 1 || err != nil || h.verifyCalls != 0 ||
		!strings.Contains(h.out.String(), "module lookup failed") || strings.Contains(h.out.String(), "Updated") {
		t.Fatalf("version = %d, %v; verify %d, output:\n%s", code, err, h.verifyCalls, h.out.String())
	}
}

// AS-97: a post-tag development build is up to date against the prior release.
func TestVersionDevelopmentBuildNewerThanLatestAS97(t *testing.T) {
	h := setupVersion(t, "v0.6.1-0.20261116010203-abc123456789", "v0.6.0")
	code, err := Run([]string{"version"})
	if code != 0 || err != nil || h.installCalls != 0 || !strings.Contains(h.out.String(), "You're on the latest release (v0.6.0).") {
		t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
	}
}

// AS-98: no other command or dashboard action invokes the update machinery.
func TestOtherCommandsNeverCheckForUpdatesAS98(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"list"}, {"status"}, {"check"}, {"start"}, {"stop"}, {"stop", "--all"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := &fake{name: "docker", states: states(target.Stopped), checksOK: true}
			setup(t, true, false, f)
			h := setupVersion(t, "v0.6.0", "v0.7.0")
			_, _ = Run(args)
			if h.checkCalls != 0 || h.pathCalls != 0 || h.installCalls != 0 {
				t.Fatalf("%v made update calls: check %d, path %d, install %d", args, h.checkCalls, h.pathCalls, h.installCalls)
			}
		})
	}

	f := &fake{name: "docker", states: states(target.Stopped), checksOK: true}
	setup(t, true, false, f)
	h := setupVersion(t, "v0.6.0", "v0.7.0")
	cfg, _ := loadConfig("")
	entries, _ := buildTargets(cfg)
	ctl := &controller{cfg: cfg, entries: entries}
	ctl.Snapshot()
	ctl.Run(ui.Check, "docker")
	if h.checkCalls != 0 || h.pathCalls != 0 || h.installCalls != 0 {
		t.Fatalf("dashboard made update calls: check %d, path %d, install %d", h.checkCalls, h.pathCalls, h.installCalls)
	}
}

// AS-99: successful installation still fails closed unless the installed file verifies.
func TestVersionVerificationFailureAS99(t *testing.T) {
	tests := []struct {
		name      string
		installed version.Info
		err       error
		want      string
	}{
		{"missing", version.Info{}, os.ErrNotExist, "go install finished, but /home/me/go/bin/dockguard doesn't exist; not updated."},
		{"unreadable", version.Info{}, errors.New("invalid executable"), "go install finished, but /home/me/go/bin/dockguard can't be verified: invalid executable."},
		{"no build info", version.Info{}, errors.New("no build information"), "go install finished, but /home/me/go/bin/dockguard can't be verified: no build information."},
		{"wrong version", version.Info{Version: "v0.6.9"}, nil, "go install finished, but /home/me/go/bin/dockguard reports v0.6.9, not v0.7.0."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := setupVersion(t, "v0.6.0", "v0.7.0")
			setVersionInput(h, "y\n")
			h.installed, h.installedErr = tt.installed, tt.err
			code, err := Run([]string{"version"})
			if code != 1 || err != nil || !strings.Contains(h.out.String(), tt.want) || strings.Contains(h.out.String(), "Updated to") {
				t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
			}
		})
	}
}

func TestVersionUnknownDoesNotOfferUpdate(t *testing.T) {
	h := setupVersion(t, "", "v0.7.0")
	code, err := Run([]string{"version"})
	if code != 0 || err != nil || h.installCalls != 0 ||
		!strings.Contains(h.out.String(), "DockGuard (version unknown)") ||
		!strings.Contains(h.out.String(), "Latest release: v0.7.0.") || strings.Contains(h.out.String(), "[y/N]") {
		t.Fatalf("version = %d, %v, output:\n%s", code, err, h.out.String())
	}
}
