package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
)

type fakes struct {
	running bool
	err     error
	opened  string
	quit    string
}

func install(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{}
	origRunning, origOpen, origQuit := processRunning, openApp, quitApp
	t.Cleanup(func() { processRunning, openApp, quitApp = origRunning, origOpen, origQuit })
	processRunning = func(string) (bool, error) { return f.running, f.err }
	openApp = func(name string) error { f.opened = name; return nil }
	quitApp = func(name string) error { f.quit = name; return nil }
	return f
}

func xcode(paths ...string) config.Target {
	return config.Target{Name: "xcode", Kind: config.KindApp, FailIfAlreadyRunning: true,
		App: &config.App{Name: "Xcode", Process: "Xcode", SymlinkPaths: paths}}
}

// linkOntoDrive makes home/<name> a symlink to drive/<name>.
func linkOntoDrive(t *testing.T, drive, name string) string {
	t.Helper()
	dest := filepath.Join(drive, name)
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), name)
	if err := os.Symlink(dest, link); err != nil {
		t.Fatal(err)
	}
	return link
}

// AS-11: linked paths on the mounted drive pass, and start opens the app.
func TestChecksPassAndStartOpensApp(t *testing.T) {
	f := install(t)
	drive := t.TempDir()
	tg := New(xcode(linkOntoDrive(t, drive, "DerivedData"), linkOntoDrive(t, drive, "Archives")), drive)

	if items := tg.Checks(); !checks.AllOK(items) || len(items) != 4 {
		t.Fatalf("Checks() = %#v", items)
	}
	if err := tg.Start(); err != nil || f.opened != "Xcode" {
		t.Fatalf("Start() = %v, opened %q", err, f.opened)
	}
	if !strings.Contains(tg.MainPath(), " -> ") {
		t.Fatalf("MainPath() = %q, want the link and its target", tg.MainPath())
	}
}

// AS-13: a plain folder on the internal disk fails as "not on the drive".
func TestPlainFolderIsNotOnTheDrive(t *testing.T) {
	install(t)
	drive := t.TempDir()
	plain := t.TempDir()

	for _, item := range New(xcode(plain), drive).Checks() {
		if strings.HasSuffix(item.Name, "on the drive") {
			if item.OK || !strings.Contains(item.Message, "not on the drive") {
				t.Fatalf("item = %#v", item)
			}
			return
		}
	}
	t.Fatal("no on-the-drive check")
}

func TestBrokenLinkFails(t *testing.T) {
	install(t)
	drive := t.TempDir()
	link := filepath.Join(t.TempDir(), "DerivedData")
	if err := os.Symlink(filepath.Join(drive, "missing"), link); err != nil {
		t.Fatal(err)
	}

	items := New(xcode(link), drive).Checks()
	if checks.AllOK(items) || !strings.Contains(items[0].Message, "not found") {
		t.Fatalf("Checks() = %#v", items)
	}
}

func TestState(t *testing.T) {
	f := install(t)
	tg := New(xcode(), t.TempDir())

	f.running = true
	if s, _ := tg.State(); s != target.Running {
		t.Fatalf("State() = %v", s)
	}
	f.running = false
	if s, _ := tg.State(); s != target.Stopped {
		t.Fatalf("State() = %v", s)
	}
	f.err = errors.New("pgrep failed")
	if s, err := tg.State(); s != target.Unknown || err == nil {
		t.Fatalf("State() = %v, %v", s, err)
	}
}

func TestStopQuitsOnlyWhenConfirmedRunning(t *testing.T) {
	f := install(t)
	tg := New(xcode(), t.TempDir())

	f.running = false
	if stopped, err := tg.Stop(); stopped || err != nil || f.quit != "" {
		t.Fatalf("stopped app: Stop() = %v, %v; quit %q", stopped, err, f.quit)
	}

	f.err = errors.New("pgrep failed")
	if stopped, err := tg.Stop(); stopped || err == nil || f.quit != "" {
		t.Fatalf("unknown state: Stop() = %v, %v; quit %q", stopped, err, f.quit)
	}

	f.err, f.running = nil, true
	if stopped, err := tg.Stop(); !stopped || err != nil || f.quit != "Xcode" {
		t.Fatalf("running app: Stop() = %v, %v; quit %q", stopped, err, f.quit)
	}
}
