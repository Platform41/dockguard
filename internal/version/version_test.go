package version

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
)

func buildInfo(version string, settings ...string) *debug.BuildInfo {
	info := &debug.BuildInfo{GoVersion: "go1.26.1", Main: debug.Module{Version: version}}
	for i := 0; i+1 < len(settings); i += 2 {
		info.Settings = append(info.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return info
}

func TestCurrentReleaseFormattingAS88(t *testing.T) {
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	readBuildInfo = func() (*debug.BuildInfo, bool) { return buildInfo("v0.6.0"), true }

	got := Current().String()
	want := "DockGuard v0.6.0 (go1.26.1, " + runtimePlatform() + ")"
	if got != want {
		t.Fatalf("Current = %q, want %q", got, want)
	}
}

// AS-89: pseudo-versions identify development builds, including VCS commit and modification state.
func TestCurrentDevelopmentFormattingAS89(t *testing.T) {
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	pseudo := "v0.6.1-0.20261116010203-abc123456789"
	for _, tt := range []struct {
		name     string
		settings []string
		want     string
	}{
		{"clean", []string{"vcs.revision", "abc123456789"}, "DockGuard " + pseudo + " (development build, commit abc123456789)"},
		{"modified", []string{"vcs.revision", "abc123456789", "vcs.modified", "true"}, "DockGuard " + pseudo + " (development build, commit abc123456789, modified)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readBuildInfo = func() (*debug.BuildInfo, bool) { return buildInfo(pseudo, tt.settings...), true }
			if got := Current().String(); got != tt.want {
				t.Fatalf("Current = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCurrentUnknownVersion(t *testing.T) {
	original := readBuildInfo
	t.Cleanup(func() { readBuildInfo = original })
	for _, info := range []*debug.BuildInfo{nil, buildInfo("(devel)")} {
		readBuildInfo = func() (*debug.BuildInfo, bool) { return info, info != nil }
		if got := Current().String(); got != "DockGuard (version unknown)" {
			t.Fatalf("Current = %q", got)
		}
	}
}

// AS-97: a pseudo-version after a tag is newer than that tag but older than the next release.
func TestNewerSemanticVersionsAS97(t *testing.T) {
	tests := []struct {
		current, latest string
		want            bool
	}{
		{"v0.6.0", "v0.6.0", false},
		{"v0.6.0", "v0.7.0", true},
		{"v0.7.0", "v0.6.0", false},
		{"v0.6.1-0.20261116010203-abc123456789", "v0.6.0", false},
		{"v0.6.1-0.20261116010203-abc123456789", "v0.7.0", true},
		{"v0.6.1-0.20261116010203-abc123456789", "v0.6.1", true},
		{"v0.7.0-rc.1", "v0.7.0", true},
		{"v0.7.0-rc.2", "v0.7.0-rc.10", true},
		{"v0.7.0+build.2", "v0.7.0+build.1", false},
		{"", "v0.7.0", false},
		{"v0.6.0", "not-a-version", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.current, tt.latest); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

type recordedGoCommand struct {
	args []string
	dir  string
	env  []string
}

func installGoFake(t *testing.T, responses []struct {
	output string
	code   int
	err    error
}, calls *[]recordedGoCommand) {
	t.Helper()
	original := runDetachedCommand
	t.Cleanup(func() { runDetachedCommand = original })
	*calls = nil
	runDetachedCommand = func(_ context.Context, cmd *exec.Cmd, _ io.Writer) (string, int, error) {
		if len(*calls) >= len(responses) {
			t.Fatalf("unexpected go command: %v", cmd.Args)
		}
		entries, err := os.ReadDir(cmd.Dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("go command directory %q is not fresh and empty: %v, %v", cmd.Dir, entries, err)
		}
		if !reflect.DeepEqual(cmd.Args[:1], []string{"go"}) {
			t.Fatalf("command = %v", cmd.Args)
		}
		*calls = append(*calls, recordedGoCommand{args: append([]string(nil), cmd.Args...), dir: cmd.Dir, env: append([]string(nil), cmd.Env...)})
		response := responses[len(*calls)-1]
		return response.output, response.code, response.err
	}
}

func assertIsolatedGoCalls(t *testing.T, calls []recordedGoCommand) {
	t.Helper()
	seen := map[string]bool{}
	for _, call := range calls {
		if seen[call.dir] {
			t.Errorf("directory reused between go commands: %q", call.dir)
		}
		seen[call.dir] = true
		if !hasEnv(call.env, "GOWORK=off") {
			t.Errorf("go environment lacks GOWORK=off: %v", call.env)
		}
		if _, err := os.Stat(call.dir); !os.IsNotExist(err) {
			t.Errorf("temporary directory %q was not removed: %v", call.dir, err)
		}
	}
}

func hasEnv(env []string, want string) bool {
	for _, value := range env {
		if value == want {
			return true
		}
	}
	return false
}

// AS-104: latest checks use separate fresh temporary directories with GOWORK disabled.
func TestLatestIsolationAndJSONAS104(t *testing.T) {
	var calls []recordedGoCommand
	installGoFake(t, []struct {
		output string
		code   int
		err    error
	}{{output: "/dev/null\n"}, {output: "{\"Path\":\"github.com/platform41/dockguard\",\"Version\":\"v0.7.0\"}\n"}}, &calls)

	latest, err := Latest(context.Background())
	if err != nil || latest.Version != "v0.7.0" {
		t.Fatalf("Latest = %#v, %v", latest, err)
	}
	if len(calls) != 2 || !reflect.DeepEqual(calls[1].args, []string{"go", "list", "-m", "-json", modulePath + "@latest"}) {
		t.Fatalf("go calls = %#v", calls)
	}
	assertIsolatedGoCalls(t, calls)
}

// AS-104: a module found above the temporary directory skips go list.
func TestLatestRejectsNonIsolatedModuleAS104(t *testing.T) {
	var calls []recordedGoCommand
	installGoFake(t, []struct {
		output string
		code   int
		err    error
	}{{output: "/tmp/go.mod\n"}}, &calls)

	_, err := Latest(context.Background())
	if !errors.Is(err, ErrModuleIsolation) || len(calls) != 1 {
		t.Fatalf("Latest error = %v, calls = %d", err, len(calls))
	}
	assertIsolatedGoCalls(t, calls)
}

// AS-94 and AS-104: failures and timeouts are reported and still remove the temporary directory.
func TestLatestFailureAndTimeoutCleanupAS94AS104(t *testing.T) {
	for _, response := range []struct {
		output string
		code   int
		err    error
	}{{output: "module lookup disabled\n", code: 1}, {err: context.DeadlineExceeded}} {
		var calls []recordedGoCommand
		installGoFake(t, []struct {
			output string
			code   int
			err    error
		}{response}, &calls)
		_, err := Latest(context.Background())
		if err == nil {
			t.Fatal("Latest error = nil")
		}
		if response.code != 0 && !strings.Contains(err.Error(), "module lookup disabled") {
			t.Fatalf("Latest error = %v", err)
		}
		if response.err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Latest error = %v", err)
		}
		assertIsolatedGoCalls(t, calls)
	}
}

// AS-93: failure to start go is distinguishable from an unsuccessful release check.
func TestLatestGoNotInstalledAS93(t *testing.T) {
	var calls []recordedGoCommand
	installGoFake(t, []struct {
		output string
		code   int
		err    error
	}{{code: -1, err: exec.ErrNotFound}}, &calls)
	_, err := Latest(context.Background())
	if !errors.Is(err, ErrGoNotInstalled) {
		t.Fatalf("Latest error = %v", err)
	}
}

func TestInstallPath(t *testing.T) {
	for _, tt := range []struct {
		name, output, want string
	}{
		{"GOBIN", "/custom/bin\n/home/me/go\n", "/custom/bin/dockguard"},
		{"GOPATH", "\n/home/me/go\n", "/home/me/go/bin/dockguard"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []recordedGoCommand
			installGoFake(t, []struct {
				output string
				code   int
				err    error
			}{{output: tt.output}}, &calls)
			path, err := InstallPath()
			if err != nil || path != tt.want {
				t.Fatalf("InstallPath = %q, %v, want %q", path, err, tt.want)
			}
			if len(calls) != 1 || !reflect.DeepEqual(calls[0].args, []string{"go", "env", "GOBIN", "GOPATH"}) {
				t.Fatalf("go calls = %#v", calls)
			}
			assertIsolatedGoCalls(t, calls)
		})
	}
}

// AS-90 and AS-104: installation uses the pinned version in an isolated directory and streams output.
func TestInstallPinnedAndStreamingAS90AS104(t *testing.T) {
	original := runDetachedCommand
	t.Cleanup(func() { runDetachedCommand = original })
	var dir string
	runDetachedCommand = func(_ context.Context, cmd *exec.Cmd, stream io.Writer) (string, int, error) {
		dir = cmd.Dir
		want := []string{"go", "install", commandPath + "@v0.7.0"}
		if !reflect.DeepEqual(cmd.Args, want) || !hasEnv(cmd.Env, "GOWORK=off") {
			t.Fatalf("install = %v, env %v", cmd.Args, cmd.Env)
		}
		_, _ = io.WriteString(stream, "downloading\n")
		return "", 0, nil
	}
	var output strings.Builder
	if err := Install(context.Background(), "v0.7.0", &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "downloading\n" {
		t.Fatalf("streamed output = %q", output.String())
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("temporary install directory remains: %v", err)
	}
}

func TestInstalledReadsInstalledFile(t *testing.T) {
	original := readInstalledInfo
	t.Cleanup(func() { readInstalledInfo = original })
	var gotPath string
	readInstalledInfo = func(path string) (*debug.BuildInfo, error) {
		gotPath = path
		return buildInfo("v0.7.0"), nil
	}
	info, err := Installed("/tmp/dockguard")
	if err != nil || info.Version != "v0.7.0" || gotPath != "/tmp/dockguard" {
		t.Fatalf("Installed = %#v, %v; path %q", info, err, gotPath)
	}
}
