package platform

import (
	"errors"
	"strings"
	"testing"
)

func TestOpenAppRunsOpenA(t *testing.T) {
	original := runApp
	t.Cleanup(func() { runApp = original })

	var got []string
	runApp = func(name string, args ...string) error { got = append([]string{name}, args...); return nil }

	if err := OpenApp("Xcode"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "open -a Xcode" {
		t.Fatalf("ran %v", got)
	}
}

func TestOpenAppWrapsError(t *testing.T) {
	original := runApp
	t.Cleanup(func() { runApp = original })
	runApp = func(string, ...string) error { return errors.New("application not found") }

	if err := OpenApp("Xcode"); err == nil || !strings.Contains(err.Error(), "open Xcode") {
		t.Fatalf("err = %v", err)
	}
}

func TestQuitAppAsksToQuit(t *testing.T) {
	original := runApp
	t.Cleanup(func() { runApp = original })

	var got []string
	runApp = func(name string, args ...string) error { got = append([]string{name}, args...); return nil }

	if err := QuitApp("Xcode"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != `osascript -e quit app "Xcode"` {
		t.Fatalf("ran %v", got)
	}
}

func TestProcessRunning(t *testing.T) {
	original := runPgrep
	t.Cleanup(func() { runPgrep = original })

	tests := []struct {
		code    int
		err     error
		want    bool
		wantErr bool
	}{
		{0, nil, true, false},
		{1, errors.New("exit status 1"), false, false},
		{2, errors.New("exit status 2"), false, true},
		{-1, errors.New("executable file not found"), false, true},
	}
	for _, tt := range tests {
		runPgrep = func(string, ...string) (int, error) { return tt.code, tt.err }
		got, err := ProcessRunning("Xcode")
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Fatalf("code %d: ProcessRunning = %v, %v", tt.code, got, err)
		}
	}
}

// Review on PR #6: pgrep -x takes a regular expression, so "DG+Probe" must be escaped or a running
// app reads as stopped.
func TestProcessRunningEscapesRegexCharacters(t *testing.T) {
	original := runPgrep
	t.Cleanup(func() { runPgrep = original })

	var got []string
	runPgrep = func(name string, args ...string) (int, error) { got = args; return 0, nil }

	if _, err := ProcessRunning("DG+Probe (1).app"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "-x" || got[1] != `DG\+Probe \(1\)\.app` {
		t.Fatalf("pgrep args = %q", got)
	}
}
