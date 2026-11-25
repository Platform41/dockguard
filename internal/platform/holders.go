package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Holder is a process with files open on a volume.
type Holder struct {
	Name string
	PID  int
	UID  int // -1 when lsof does not report a UID
	User string
	PPID int
}

func (h Holder) String() string { return fmt.Sprintf("%s (PID %d)", h.Name, h.PID) }

// HolderScanTimeout keeps a dashboard refresh bounded below its three-second interval.
var HolderScanTimeout = 2 * time.Second

// Swappable for tests.
var runLsof = runLsofWithTimeout

func runLsofWithTimeout(name string, args ...string) (output string, code int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), HolderScanTimeout)
	defer cancel()
	out, runErr := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if ctx.Err() != nil {
		return string(out), -1, fmt.Errorf("%s timed out after %s", name, HolderScanTimeout)
	}
	if runErr == nil {
		return string(out), 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return string(out), exitErr.ExitCode(), nil
	}
	return string(out), -1, runErr
}

// Holders lists the processes with files open on the volume mounted at mountPath, using lsof's
// field output. lsof exits 1 when it finds nothing, which is an empty list, not an error.
func Holders(mountPath string) ([]Holder, error) {
	output, code, err := runLsof("lsof", "+c", "0", "-FpcuLR", "+f", "--", mountPath)
	if err != nil {
		return nil, fmt.Errorf("lsof: %w", err)
	}
	if strings.TrimSpace(output) == "" {
		if code == 0 || code == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof inspection failed: exit %d", code)
	}
	holders := parseLsof(output)
	if code != 0 || strings.Contains(output, "lsof:") {
		return holders, fmt.Errorf("lsof inspection incomplete: %s", strings.TrimSpace(output))
	}
	return holders, nil
}

// parseLsof reads process fields in the order lsof emits them. A new PID starts a
// fresh record, even if the previous record had fields that this one omits.
func parseLsof(output string) []Holder {
	byPID := map[int]Holder{}
	current := Holder{UID: -1}
	save := func() {
		if current.PID <= 0 || current.Name == "lsof" {
			return
		}
		if current.Name == "" {
			current.Name = "unknown process"
		}
		if current.User == "" && current.UID >= 0 {
			current.User = strconv.Itoa(current.UID)
		}
		byPID[current.PID] = current
	}
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		value := line[1:]
		switch line[0] {
		case 'p':
			save()
			pid, err := strconv.ParseInt(value, 10, 32)
			current = Holder{UID: -1}
			if err == nil {
				current.PID = int(pid)
			}
		case 'c':
			current.Name = value
		case 'u':
			if uid, err := strconv.Atoi(value); err == nil && uid >= 0 {
				current.UID = uid
			}
		case 'L':
			current.User = value
		case 'R':
			current.PPID, _ = strconv.Atoi(value)
		}
	}
	save()

	holders := make([]Holder, 0, len(byPID))
	for _, h := range byPID {
		holders = append(holders, h)
	}
	sort.Slice(holders, func(i, j int) bool { return holders[i].PID < holders[j].PID })
	return holders
}

func describeHolders(holders []Holder) string {
	names := make([]string, len(holders))
	for i, h := range holders {
		names[i] = h.String()
	}
	return strings.Join(names, ", ")
}

// DescribeHolders formats holders for app and platform error messages.
func DescribeHolders(holders []Holder) string { return describeHolders(holders) }
