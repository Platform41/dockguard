package platform

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseLsof(t *testing.T) {
	out := "p33061\ncGoogle Chrome\nf12\nf13\np18474\ncmdworker_shared\nf3\np33061\ncGoogle Chrome\nf14\np99\nclsof\nfcwd\n"
	got := parseLsof(out)
	if len(got) != 2 || got[0] != (Holder{"mdworker_shared", 18474}) || got[1] != (Holder{"Google Chrome", 33061}) {
		t.Fatalf("parseLsof = %#v", got)
	}
}

func TestHoldersNothingFound(t *testing.T) {
	original := runLsof
	t.Cleanup(func() { runLsof = original })
	runLsof = func(string, ...string) (string, int, error) { return "", 1, nil }

	if h, err := Holders("/Volumes/X"); err != nil || len(h) != 0 {
		t.Fatalf("Holders = %v, %v", h, err)
	}
}

func TestHoldersUsesFieldOutputWithFullNames(t *testing.T) {
	original := runLsof
	t.Cleanup(func() { runLsof = original })

	var args []string
	runLsof = func(name string, a ...string) (string, int, error) { args = a; return "p1\ncx\n", 0, nil }
	if _, err := Holders("/Volumes/X"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(args, " "); got != "+c 0 -Fpc +f -- /Volumes/X" {
		t.Fatalf("lsof args = %q", got)
	}
}

func TestHoldersReturnsParsedProcessesAndInspectionError(t *testing.T) {
	original := runLsof
	t.Cleanup(func() { runLsof = original })
	runLsof = func(string, ...string) (string, int, error) {
		return "lsof: WARNING: incomplete results\np42\nczsh\nfcwd\n", 1, nil
	}

	holders, err := Holders("/Volumes/X")
	if len(holders) != 1 || holders[0] != (Holder{"zsh", 42}) || err == nil ||
		!strings.Contains(err.Error(), "incomplete results") {
		t.Fatalf("Holders = %#v, %v", holders, err)
	}
}

func TestLsofScanTimesOut(t *testing.T) {
	original := HolderScanTimeout
	t.Cleanup(func() { HolderScanTimeout = original })
	HolderScanTimeout = 10 * time.Millisecond

	if _, _, err := runLsofWithTimeout("sh", "-c", "sleep 1"); err == nil ||
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error = %v", err)
	}
}

// The v0.3.0 live check: a busy eject now names what holds the drive.
func TestEjectVolumeNamesHoldersWhenBusy(t *testing.T) {
	origLook, origRun, origSleep, origHolders := lookPath, runCommand, sleep, holdersOf
	t.Cleanup(func() { lookPath, runCommand, sleep, holdersOf = origLook, origRun, origSleep, origHolders })
	lookPath = func(string) (string, error) { return "/usr/sbin/diskutil", nil }
	runCommand = func(string, ...string) error {
		return errors.New("Unmount of disk7 failed: at least one volume could not be unmounted")
	}
	sleep = func(time.Duration) {}

	holdersOf = func(string) ([]Holder, error) { return []Holder{{"Google Chrome", 33061}}, nil }
	err := EjectVolume("/Volumes/X")
	if err == nil || !strings.Contains(err.Error(), "still using the drive: Google Chrome (PID 33061)") {
		t.Fatalf("err = %v", err)
	}

	holdersOf = func(string) ([]Holder, error) { return nil, nil }
	err = EjectVolume("/Volumes/X")
	if err == nil || !strings.Contains(err.Error(), "close Finder previews") {
		t.Fatalf("fallback err = %v", err)
	}

	holdersOf = func(string) ([]Holder, error) { return nil, errors.New("lsof timed out") }
	err = EjectVolume("/Volumes/X")
	if err == nil || !strings.Contains(err.Error(), "couldn't inspect") || !strings.Contains(err.Error(), "lsof timed out") {
		t.Fatalf("inspection err = %v", err)
	}
}
