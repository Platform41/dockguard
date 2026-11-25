package platform

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Process identifies a running process and the executable currently loaded in it.
type Process struct {
	Name    string
	Path    string
	Started time.Time
	UID     int
	PPID    int
}

// MatchesCommand compares the full lsof name with the kernel command. Darwin
// limits P_comm to 16 bytes, so use the executable basename to corroborate a
// longer name with that same prefix.
func (p Process) MatchesCommand(name string) bool {
	const maxKernelCommand = 16
	return p.Name != "" && (p.Name == name ||
		(len(p.Name) == maxKernelCommand && strings.HasPrefix(name, p.Name) &&
			strings.HasPrefix(filepath.Base(p.Path), name)))
}

// Swappable for tests.
var (
	InspectProcess = inspectProcess
	Signal         = signal
	Ancestors      = ancestors
)

func signal(pid int, sig syscall.Signal) error {
	if pid <= 0 || pid > math.MaxInt32 {
		return fmt.Errorf("invalid process ID %d", pid)
	}
	return syscall.Kill(pid, sig)
}

func ancestors(pid int) ([]int, error) {
	seen := map[int]bool{pid: true}
	var parents []int
	for pid > 1 {
		parent, err := processParent(pid)
		if err != nil {
			return parents, err
		}
		if parent <= 0 || seen[parent] {
			return parents, fmt.Errorf("cannot determine the parent of PID %d", pid)
		}
		parents = append(parents, parent)
		seen[parent] = true
		pid = parent
	}
	return parents, nil
}
