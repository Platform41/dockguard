package platform

import (
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAncestorsWalkThroughRootLogin(t *testing.T) {
	old := readKinfoProc
	t.Cleanup(func() { readKinfoProc = old })
	processes := map[int]struct{ parent, uid int }{
		40: {30, 501}, // DockGuard
		30: {20, 501}, // Shell
		20: {10, 0},   // login
		10: {1, 501},  // Terminal
	}
	readKinfoProc = func(name string, args ...int) (*unix.KinfoProc, error) {
		if name != "kern.proc.pid" || len(args) != 1 {
			t.Fatalf("unexpected sysctl %q %v", name, args)
		}
		process, ok := processes[args[0]]
		if !ok {
			t.Fatalf("unexpected PID %d", args[0])
		}
		info := &unix.KinfoProc{}
		info.Eproc.Ppid = int32(process.parent)
		info.Eproc.Ucred.Uid = uint32(process.uid)
		return info, nil
	}
	got, err := ancestors(40)
	if want := []int{30, 20, 10, 1}; err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ancestors = %v, %v; want %v", got, err, want)
	}
}

func TestProcessExecutableReadsOnlyAbsoluteExecutablePath(t *testing.T) {
	for _, tt := range []struct{ data, want string }{
		{"\x01\x00\x00\x00/bin/editor\x00\x00argv0\x00ENV=value\x00", "/bin/editor"},
		{"\x01\x00\x00\x00relative\x00", ""},
		{"\x01\x00\x00\x00/bin/editor", ""},
		{"\x01\x00\x00", ""},
	} {
		if got := processExecutable([]byte(tt.data)); got != tt.want {
			t.Errorf("processExecutable(%q) = %q, want %q", tt.data, got, tt.want)
		}
	}
}

func TestProcessMatchesLongCommand(t *testing.T) {
	process := Process{Name: "long-executable-", Path: "/tmp/long-executable-name-for-a-desktop-app"}
	for _, tt := range []struct {
		name string
		want bool
	}{
		{"long-executable-", true},
		{"long-executable-name-for-a-desktop-app", true},
		{"long-executable-name-for-a-desk", true},
		{"long-executable-other", false},
		{"long-executable", false},
	} {
		if got := process.MatchesCommand(tt.name); got != tt.want {
			t.Errorf("MatchesCommand(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
