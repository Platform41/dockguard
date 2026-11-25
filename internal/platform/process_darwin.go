package platform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const (
	kernProcPID = "kern.proc.pid"
	zombieState = 5 // Darwin's SZOMB, awaiting collection by its parent.
)

var readKinfoProc = unix.SysctlKinfoProc

func processInfo(pid int) (*unix.KinfoProc, error) {
	info, err := readKinfoProc(kernProcPID, pid)
	if errors.Is(err, unix.EIO) {
		// SysctlKinfoProc returns EIO for a short result, including an empty
		// record for a PID that no longer exists. Confirm that it is empty.
		if data, readErr := unix.SysctlRaw(kernProcPID, pid); readErr == nil && len(data) == 0 {
			return nil, os.ErrProcessDone
		}
	}
	if errors.Is(err, unix.ESRCH) {
		return nil, os.ErrProcessDone
	}
	if err != nil {
		return nil, fmt.Errorf("inspect PID %d: %w", pid, err)
	}
	if info.Proc.P_stat == zombieState {
		return nil, os.ErrProcessDone
	}
	return info, nil
}

func processParent(pid int) (int, error) {
	// This table includes parents owned by another user, such as login.
	info, err := processInfo(pid)
	if err != nil {
		return 0, err
	}
	return int(info.Eproc.Ppid), nil
}

func inspectProcess(pid int) (Process, error) {
	before, err := processInfo(pid)
	if err != nil {
		return Process{}, err
	}
	data, pathErr := unix.SysctlRaw("kern.procargs2", pid)
	// Check the identity again after reading the separate executable path,
	// even if that read failed: the process may have exited in the meantime.
	after, err := processInfo(pid)
	if err != nil {
		return Process{}, err
	}
	if before.Proc.P_starttime != after.Proc.P_starttime || before.Proc.P_comm != after.Proc.P_comm ||
		before.Eproc.Ucred.Uid != after.Eproc.Ucred.Uid {
		return Process{}, os.ErrProcessDone
	}
	if pathErr != nil {
		return Process{}, fmt.Errorf("inspect executable for PID %d: %w", pid, pathErr)
	}
	path := processExecutable(data)
	if path == "" {
		return Process{}, fmt.Errorf("inspect executable for PID %d: missing absolute path", pid)
	}

	start := after.Proc.P_starttime
	return Process{
		Name: unix.ByteSliceToString(after.Proc.P_comm[:]), Path: path,
		Started: time.Unix(start.Sec, int64(start.Usec)*1000),
		UID:     int(after.Eproc.Ucred.Uid), PPID: int(after.Eproc.Ppid),
	}, nil
}

// kern.procargs2 starts with a C int argument count followed by the
// NUL-terminated executable path. The arguments and environment follow it.
func processExecutable(data []byte) string {
	const argCountSize = 4
	if len(data) <= argCountSize {
		return ""
	}
	path, _, ok := bytes.Cut(data[argCountSize:], []byte{0})
	if !ok || !filepath.IsAbs(string(path)) {
		return ""
	}
	return string(path)
}
