package postgres

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/target"
)

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		output string
		want   string
		ok     bool
	}{
		{"pg_ctl (PostgreSQL) 17.11 (Homebrew)\n", "17", true},
		{"pg_ctl (PostgreSQL) 18.6\n", "18", true},
		{"pg_ctl (PostgreSQL) 10.0\n", "10", true},
		{"pg_ctl (PostgreSQL) 18devel\n", "18", true},
		{"pg_ctl (PostgreSQL) 17beta1\n", "17", true},
		{"pg_ctl (PostgreSQL) 9.6.24\n", "9.6", true},
		{"pg_ctl (PostgreSQL) 9.5.25\n", "9.5", true},
		{"pg_ctl (PostgreSQL) 9\n", "", false},
		{"command not found\n", "", false},
	}
	for _, tt := range tests {
		got, ok := MajorVersion(tt.output)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("MajorVersion(%q) = %q, %v; want %q, %v", tt.output, got, ok, tt.want, tt.ok)
		}
	}
}

// call records one run of a command.
type call struct {
	name string
	args []string
}

type fakes struct {
	calls      []call
	version    string
	statusCode int
	statusErr  error
	cmdCode    int
	alive      map[int]bool
	portBusy   bool
	detached   bool
}

func install(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{version: "pg_ctl (PostgreSQL) 17.11 (Homebrew)\n", statusCode: statusNotRunning, alive: map[int]bool{}}
	origRun, origDetached, origAlive, origPort := run, runDetached, processAlive, portInUse
	t.Cleanup(func() { run, runDetached, processAlive, portInUse = origRun, origDetached, origAlive, origPort })

	run = func(name string, args ...string) (string, int, error) {
		f.calls = append(f.calls, call{name, args})
		switch {
		case len(args) == 1 && args[0] == "--version":
			return f.version, 0, nil
		case len(args) == 3 && args[2] == "status":
			return "", f.statusCode, f.statusErr
		default:
			return "", f.cmdCode, nil
		}
	}
	// start must go through runDetached; record it the same way.
	runDetached = func(name string, args ...string) (string, int, error) {
		f.calls = append(f.calls, call{name, append([]string{}, args...)})
		f.detached = true
		return "", f.cmdCode, nil
	}
	processAlive = func(pid int) bool { return f.alive[pid] }
	portInUse = func(int) bool { return f.portBusy }
	return f
}

// cluster makes a drive with a data_dir holding PG_VERSION, and a bin_dir with pg_ctl and postgres.
func cluster(t *testing.T, pgVersion string, port int) (*Target, string) {
	t.Helper()
	drive := t.TempDir()
	dataDir := filepath.Join(drive, "pgdata", "myapp")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte(pgVersion+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	for _, bin := range []string{"pg_ctl", "postgres"} {
		if err := os.WriteFile(filepath.Join(binDir, bin), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Target{Name: "myapp-db", Kind: config.KindPostgres, MinimumFreeSpaceGB: 1, FailIfAlreadyRunning: true,
		Postgres: &config.Postgres{DataDir: dataDir, BinDir: binDir, Port: port, SocketDir: "/tmp", LogFile: filepath.Join(drive, "pgdata", "myapp.log")}}
	return New(cfg, drive), dataDir
}

func find(items []checks.Item, name string) checks.Item {
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	return checks.Item{Name: "missing: " + name}
}

func TestChecksPass(t *testing.T) {
	install(t)
	tg, _ := cluster(t, "17", 55432)
	if items := tg.Checks(); !checks.AllOK(items) {
		t.Fatalf("Checks() = %#v", items)
	}
}

// AS-16: PG_VERSION 17 against an 18 pg_ctl fails and shows both.
func TestVersionMismatchFails(t *testing.T) {
	f := install(t)
	f.version = "pg_ctl (PostgreSQL) 18.6\n"
	tg, _ := cluster(t, "17", 0)

	item := find(tg.Checks(), "PG_VERSION matches bin_dir")
	if item.OK || !strings.Contains(item.Message, "17") || !strings.Contains(item.Message, "18") {
		t.Fatalf("item = %#v", item)
	}
}

// AS-26: a 9.6 cluster passes with 9.6 tools and fails with 9.5 tools.
func TestPre10Versions(t *testing.T) {
	f := install(t)
	tg, _ := cluster(t, "9.6", 0)

	f.version = "pg_ctl (PostgreSQL) 9.6.24\n"
	if item := find(tg.Checks(), "PG_VERSION matches bin_dir"); !item.OK {
		t.Fatalf("9.6 tools: %#v", item)
	}
	f.version = "pg_ctl (PostgreSQL) 9.5.25\n"
	if item := find(tg.Checks(), "PG_VERSION matches bin_dir"); item.OK {
		t.Fatalf("9.5 tools: %#v", item)
	}
}

// AS-23: a postmaster.pid naming a dead process fails and says how to clear it.
func TestStalePidFileFails(t *testing.T) {
	install(t)
	tg, dataDir := cluster(t, "17", 0)
	if err := os.WriteFile(filepath.Join(dataDir, "postmaster.pid"), []byte("424242\n"+dataDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	item := find(tg.Checks(), "no stale postmaster.pid")
	if item.OK || !strings.Contains(item.Message, "424242") || !strings.Contains(item.Message, "remove the file") {
		t.Fatalf("item = %#v", item)
	}
}

func TestLivePidFileUsesItsOwnPort(t *testing.T) {
	f := install(t)
	f.alive[4242] = true
	f.portBusy = true
	tg, dataDir := cluster(t, "17", 55432)
	if err := os.WriteFile(filepath.Join(dataDir, "postmaster.pid"), []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	items := tg.Checks()
	if item := find(items, "no stale postmaster.pid"); !item.OK {
		t.Fatalf("pid item = %#v", item)
	}
	if item := find(items, "port 55432 free"); !item.OK || item.Message != "used by this cluster" {
		t.Fatalf("port item = %#v", item)
	}
}

func TestPortUsedByAnotherProcessFails(t *testing.T) {
	f := install(t)
	f.portBusy = true
	tg, _ := cluster(t, "17", 55432)

	if item := find(tg.Checks(), "port 55432 free"); item.OK {
		t.Fatalf("item = %#v", item)
	}
}

func TestNoPortCheckWithoutPort(t *testing.T) {
	install(t)
	tg, _ := cluster(t, "17", 0)
	for _, item := range tg.Checks() {
		if strings.HasPrefix(item.Name, "port ") {
			t.Fatalf("unexpected %#v", item)
		}
	}
}

func TestDataDirOffTheDriveFails(t *testing.T) {
	install(t)
	tg, _ := cluster(t, "17", 0)
	tg.drive = t.TempDir()

	if item := find(tg.Checks(), "data_dir on the drive"); item.OK {
		t.Fatalf("item = %#v", item)
	}
}

func TestState(t *testing.T) {
	f := install(t)
	tg, _ := cluster(t, "17", 0)

	tests := []struct {
		code int
		err  error
		want target.State
	}{
		{0, nil, target.Running},
		{3, nil, target.Stopped},
		{4, nil, target.Unknown},
		{-1, errors.New("no such file"), target.Unknown},
	}
	for _, tt := range tests {
		f.statusCode, f.statusErr = tt.code, tt.err
		got, err := tg.State()
		if got != tt.want || (got == target.Unknown) != (err != nil) {
			t.Fatalf("exit %d: State() = %v, %v", tt.code, got, err)
		}
	}
	if last := f.calls[len(f.calls)-1]; !strings.HasSuffix(last.name, "/pg_ctl") {
		t.Fatalf("status ran %q, want bin_dir's pg_ctl", last.name)
	}
}

// AS-15: start runs bin_dir's pg_ctl with the configured port, socket folder and log file.
func TestStartArguments(t *testing.T) {
	f := install(t)
	tg, dataDir := cluster(t, "17", 55432)

	if err := tg.Start(); err != nil {
		t.Fatal(err)
	}
	last := f.calls[len(f.calls)-1]
	got := strings.Join(last.args, " ")
	want := fmt.Sprintf("-D %s -l %s -o -p 55432 -k '/tmp' -w start", dataDir, tg.cfg.Postgres.LogFile)
	if got != want || last.name != filepath.Join(tg.cfg.Postgres.BinDir, "pg_ctl") {
		t.Fatalf("ran %s %s\nwant      %s", last.name, got, want)
	}
	if !f.detached {
		t.Fatal("start didn't use RunDetached")
	}
}

func TestStartQuotesSocketDir(t *testing.T) {
	if got := serverOptions(&config.Postgres{SocketDir: "/tmp/it's here"}); got != `-k '/tmp/it'\''s here'` {
		t.Fatalf("serverOptions = %s", got)
	}
}

func TestStartFailureIsReported(t *testing.T) {
	f := install(t)
	f.cmdCode = 1
	tg, _ := cluster(t, "17", 0)
	if err := tg.Start(); err == nil {
		t.Fatal("Start() error = nil")
	}
}

// AS-17: stopping an already-stopped cluster says so and runs no stop.
func TestStopWhenStopped(t *testing.T) {
	f := install(t)
	f.statusCode = statusNotRunning
	tg, _ := cluster(t, "17", 0)

	stopped, err := tg.Stop()
	if stopped || err != nil {
		t.Fatalf("Stop() = %v, %v", stopped, err)
	}
	for _, c := range f.calls {
		if c.args[len(c.args)-1] == "stop" {
			t.Fatalf("ran a stop: %v", c.args)
		}
	}
}

// AS-25: a cluster started by hand is running, and stop stops it in fast mode.
func TestStopRunningClusterUsesFastMode(t *testing.T) {
	f := install(t)
	f.statusCode = statusRunning
	tg, dataDir := cluster(t, "17", 0)

	stopped, err := tg.Stop()
	if !stopped || err != nil {
		t.Fatalf("Stop() = %v, %v", stopped, err)
	}
	last := f.calls[len(f.calls)-1]
	if got, want := strings.Join(last.args, " "), "-D "+dataDir+" -m fast -w stop"; got != want {
		t.Fatalf("ran %q, want %q", got, want)
	}
}

func TestStopWhenStateUnknownFails(t *testing.T) {
	f := install(t)
	f.statusCode = 4
	tg, _ := cluster(t, "17", 0)

	if stopped, err := tg.Stop(); stopped || err == nil {
		t.Fatalf("Stop() = %v, %v", stopped, err)
	}
}

func TestIsPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("can't listen:", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if !isPortInUse(port) {
		t.Fatal("isPortInUse(listening port) = false")
	}
	_ = ln.Close()
	if isPortInUse(port) {
		t.Fatal("isPortInUse(closed port) = true")
	}
}
