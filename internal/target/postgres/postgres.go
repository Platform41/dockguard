// Package postgres is the postgres target kind: one PostgreSQL cluster run directly with pg_ctl
// from a version-specific bin_dir (docs/specs/multi-target.md §4.3, D-016).
package postgres

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/platform41/dockguard/internal/checks"
	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/pathutil"
	"github.com/platform41/dockguard/internal/platform"
	"github.com/platform41/dockguard/internal/target"
)

// Swappable for tests.
var (
	run          = platform.Run
	runDetached  = platform.RunDetached
	processAlive = isProcessAlive
	portInUse    = isPortInUse
)

// pg_ctl status exit codes.
const (
	statusRunning    = 0
	statusNotRunning = 3
)

// Target is one PostgreSQL cluster.
type Target struct {
	cfg   config.Target
	drive string
}

func New(cfg config.Target, drive string) *Target {
	return &Target{cfg: cfg, drive: drive}
}

func (t *Target) Name() string { return t.cfg.Name }

func (t *Target) Kind() string { return config.KindPostgres }

func (t *Target) MainPath() string { return t.cfg.Postgres.DataDir }

// binPath and dataPath name a file inside bin_dir or data_dir. They resolve the folder first:
// filepath.Join would collapse "alias/.." before the symlink is followed and could name a file in
// a different folder from the one the filesystem reaches.
func (t *Target) binPath(name string) string {
	return filepath.Join(pathutil.Resolve(t.cfg.Postgres.BinDir), name)
}

func (t *Target) dataPath(name string) string {
	return filepath.Join(pathutil.Resolve(t.cfg.Postgres.DataDir), name)
}

func (t *Target) pgCtl() string { return t.binPath("pg_ctl") }

func (t *Target) Checks() []checks.Item {
	pg := t.cfg.Postgres
	items := []checks.Item{
		checks.PathExists("pg_ctl in bin_dir", t.pgCtl()),
		checks.PathExists("postgres in bin_dir", t.binPath("postgres")),
		checks.PathExists("data_dir exists", pg.DataDir),
		checks.UnderDrive("data_dir on the drive", pg.DataDir, t.drive),
		checks.WritableDirectory("data_dir writable", pg.DataDir),
		checks.MinimumFreeSpace(pg.DataDir, t.cfg.MinimumFreeSpaceGB),
		t.versionCheck(),
	}
	pidItem, serverAlive := t.pidCheck()
	items = append(items, pidItem)
	if pg.Port != 0 {
		items = append(items, t.portCheck(serverAlive))
	}
	return items
}

// versionCheck compares the cluster's PG_VERSION with the major version of bin_dir's pg_ctl.
func (t *Target) versionCheck() checks.Item {
	const name = "PG_VERSION matches bin_dir"
	content, err := os.ReadFile(t.dataPath("PG_VERSION"))
	if err != nil {
		return checks.Item{Name: name, OK: false, Message: err.Error()}
	}
	cluster := strings.TrimSpace(string(content))

	output, code, err := run(t.pgCtl(), "--version")
	if err != nil || code != 0 {
		return checks.Item{Name: name, OK: false, Message: fmt.Sprintf("can't run %s --version: %s", t.pgCtl(), describe(output, code, err))}
	}
	tools, ok := MajorVersion(output)
	if !ok {
		return checks.Item{Name: name, OK: false, Message: fmt.Sprintf("can't read a version from %q", strings.TrimSpace(output))}
	}

	if cluster != tools {
		return checks.Item{Name: name, OK: false, Message: fmt.Sprintf("cluster is %s but bin_dir is %s", cluster, tools)}
	}
	return checks.Item{Name: name, OK: true, Message: cluster}
}

var versionPattern = regexp.MustCompile(`\(PostgreSQL\)\s+(\d+)(?:\.(\d+))?`)

// MajorVersion reads the major version from `pg_ctl --version`. From PostgreSQL 10 the major is the
// first number (17.11 -> 17); before 10 it's the first two (9.6.24 -> 9.6), which is also what
// PG_VERSION holds for those clusters.
func MajorVersion(output string) (string, bool) {
	m := versionPattern.FindStringSubmatch(output)
	if m == nil {
		return "", false
	}
	first, err := strconv.Atoi(m[1])
	if err != nil {
		return "", false
	}
	if first >= 10 {
		return m[1], true
	}
	if m[2] == "" {
		return "", false
	}
	return m[1] + "." + m[2], true
}

// pidCheck fails on a postmaster.pid whose process isn't running (AS-23). It also reports whether
// the cluster's own server is alive, so the port check can tell its own server from another.
func (t *Target) pidCheck() (checks.Item, bool) {
	const name = "no stale postmaster.pid"
	pidFile := t.dataPath("postmaster.pid")
	content, err := os.ReadFile(pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return checks.Item{Name: name, OK: true, Message: "none"}, false
		}
		return checks.Item{Name: name, OK: false, Message: err.Error()}, false
	}

	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	pid, err := strconv.Atoi(strings.TrimSpace(firstLine))
	if err != nil || pid <= 0 {
		return checks.Item{Name: name, OK: false, Message: fmt.Sprintf("can't read a PID from %s", pidFile)}, false
	}
	if processAlive(pid) {
		return checks.Item{Name: name, OK: true, Message: fmt.Sprintf("server running as PID %d", pid)}, true
	}
	return checks.Item{
		Name:    name,
		OK:      false,
		Message: fmt.Sprintf("%s names PID %d, which isn't running; if the server is stopped, remove the file", pidFile, pid),
	}, false
}

// portCheck fails when something other than this cluster's server answers on the port.
func (t *Target) portCheck(serverAlive bool) checks.Item {
	port := t.cfg.Postgres.Port
	name := fmt.Sprintf("port %d free", port)
	if serverAlive {
		return checks.Item{Name: name, OK: true, Message: "used by this cluster"}
	}
	if portInUse(port) {
		return checks.Item{Name: name, OK: false, Message: fmt.Sprintf("another process is listening on port %d", port)}
	}
	return checks.Item{Name: name, OK: true, Message: "free"}
}

func (t *Target) State() (target.State, error) {
	output, code, err := run(t.pgCtl(), "-D", t.cfg.Postgres.DataDir, "status")
	switch {
	case err != nil:
		return target.Unknown, fmt.Errorf("%s status: %w", t.pgCtl(), err)
	case code == statusRunning:
		return target.Running, nil
	case code == statusNotRunning:
		return target.Stopped, nil
	default:
		return target.Unknown, fmt.Errorf("%s status: %s", t.pgCtl(), describe(output, code, nil))
	}
}

// Start runs pg_ctl start with only the options in the config; it never edits postgresql.conf.
// -l is always passed (the config defaults it next to data_dir), and output is captured through a
// file, so the server can't hold DockGuard's output open after pg_ctl exits.
func (t *Target) Start() error {
	pg := t.cfg.Postgres
	args := []string{"-D", pg.DataDir, "-l", pg.LogFile}
	if opts := serverOptions(pg); opts != "" {
		args = append(args, "-o", opts)
	}
	args = append(args, "-w", "start")

	output, code, err := runDetached(t.pgCtl(), args...)
	if err != nil || code != 0 {
		return fmt.Errorf("start %s: %s", t.cfg.Name, describe(output, code, err))
	}
	return nil
}

// serverOptions builds pg_ctl's -o value. pg_ctl hands it to a shell, so each value is quoted.
func serverOptions(pg *config.Postgres) string {
	var opts []string
	if pg.Port != 0 {
		opts = append(opts, "-p "+strconv.Itoa(pg.Port))
	}
	if pg.SocketDir != "" {
		opts = append(opts, "-k "+shellQuote(pg.SocketDir))
	}
	return strings.Join(opts, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Stop uses fast mode, so the next start needs no recovery. It only runs once the cluster is
// confirmed running.
func (t *Target) Stop() (bool, error) {
	state, err := t.State()
	switch state {
	case target.Stopped:
		return false, nil
	case target.Unknown:
		return false, fmt.Errorf("can't tell whether %s is running: %w", t.cfg.Name, err)
	}

	output, code, err := run(t.pgCtl(), "-D", t.cfg.Postgres.DataDir, "-m", "fast", "-w", "stop")
	if err != nil || code != 0 {
		return false, fmt.Errorf("stop %s: %s", t.cfg.Name, describe(output, code, err))
	}
	return true, nil
}

func describe(output string, code int, err error) string {
	if err != nil {
		return err.Error()
	}
	if msg := strings.TrimSpace(output); msg != "" {
		return fmt.Sprintf("exit %d: %s", code, msg)
	}
	return fmt.Sprintf("exit %d", code)
}

func isProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func isPortInUse(port int) bool {
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}
