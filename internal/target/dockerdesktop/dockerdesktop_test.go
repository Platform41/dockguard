package dockerdesktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	desktop "github.com/platform41/dockguard/internal/dockerdesktop"
	"github.com/platform41/dockguard/internal/target"
)

func setup(t *testing.T) (drive string, cfg config.Target) {
	t.Helper()
	drive = t.TempDir()
	storage := filepath.Join(drive, "DockerDesktop")
	if err := os.Mkdir(storage, 0o755); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "settings-store.json")
	if err := os.WriteFile(settings, []byte(`{"diskImageLocation": "`+storage+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return drive, config.Target{
		Name:                 "docker",
		Kind:                 config.KindDockerDesktop,
		MinimumFreeSpaceGB:   1,
		FailIfAlreadyRunning: true,
		DockerDesktop:        &config.DockerDesktop{StoragePath: storage, SettingsPath: settings},
	}
}

func names(items []checks.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Name
	}
	return out
}

func TestChecksPass(t *testing.T) {
	drive, cfg := setup(t)
	items := New(cfg, drive, false).Checks()
	if !checks.AllOK(items) {
		t.Fatalf("Checks() = %#v, want all OK", items)
	}
}

// AS-01: a v0.1 config gets exactly v0.1.2's Docker checks.
func TestLegacyChecksMatchV012(t *testing.T) {
	drive, cfg := setup(t)
	got := names(New(cfg, drive, true).Checks())
	want := []string{
		"docker storage path configured",
		"docker storage path exists",
		"docker storage path writable",
		"minimum free space",
		"docker desktop settings path configured",
		"docker desktop settings file exists",
		"docker desktop settings match storage path",
	}
	if len(got) != len(want) {
		t.Fatalf("checks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("checks = %v, want %v", got, want)
		}
	}
}

func TestCurrentFormatFailsWhenStorageIsOffTheDrive(t *testing.T) {
	_, cfg := setup(t)
	otherDrive := t.TempDir()

	for _, item := range New(cfg, otherDrive, false).Checks() {
		if item.Name == "docker storage path on the drive" {
			if item.OK {
				t.Fatalf("item = %#v, want failure", item)
			}
			return
		}
	}
	t.Fatal("no on-the-drive check")
}

func TestState(t *testing.T) {
	original := isRunning
	t.Cleanup(func() { isRunning = original })

	tests := []struct {
		running bool
		err     error
		want    target.State
	}{
		{true, nil, target.Running},
		{false, nil, target.Stopped},
		{false, errors.New("docker CLI not found"), target.Unknown},
	}
	for _, tt := range tests {
		isRunning = func() (bool, error) { return tt.running, tt.err }
		got, err := New(config.Target{DockerDesktop: &config.DockerDesktop{}}, "", false).State()
		if got != tt.want || (tt.err != nil) != (err != nil) {
			t.Fatalf("State() = %v, %v; want %v", got, err, tt.want)
		}
	}
}

func TestStartPassesOptions(t *testing.T) {
	original := start
	t.Cleanup(func() { start = original })

	var got desktop.StartOptions
	start = func(opts desktop.StartOptions) error { got = opts; return nil }

	cfg := config.Target{FailIfAlreadyRunning: true, DockerDesktop: &config.DockerDesktop{RequireCLIStartSupport: true}}
	if err := New(cfg, "", false).Start(); err != nil {
		t.Fatal(err)
	}
	if !got.FailIfAlreadyRunning || !got.RequireCLIStartSupport {
		t.Fatalf("options = %#v", got)
	}
}
