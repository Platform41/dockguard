package platform

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Holder is a process with files open on a volume.
type Holder struct {
	Name string
	PID  int
}

func (h Holder) String() string { return fmt.Sprintf("%s (PID %d)", h.Name, h.PID) }

// Swappable for tests.
var runLsof = Run

// Holders lists the processes with files open on the volume mounted at mountPath, using lsof's
// field output. lsof exits 1 when it finds nothing, which is an empty list, not an error.
func Holders(mountPath string) ([]Holder, error) {
	output, code, err := runLsof("lsof", "+c", "0", "-Fpc", "+f", "--", mountPath)
	if err != nil {
		return nil, fmt.Errorf("lsof: %w", err)
	}
	if code != 0 && strings.TrimSpace(output) == "" {
		return nil, nil
	}
	return parseLsof(output), nil
}

// parseLsof reads "p<pid>" and "c<command>" lines; other fields (such as "f<fd>") are ignored.
func parseLsof(output string) []Holder {
	byPID := map[int]string{}
	pid := 0
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			if n, err := strconv.Atoi(line[1:]); err == nil {
				pid = n
				if _, seen := byPID[pid]; !seen {
					byPID[pid] = ""
				}
			}
		case 'c':
			if pid != 0 {
				byPID[pid] = line[1:]
			}
		}
	}

	holders := make([]Holder, 0, len(byPID))
	for pid, name := range byPID {
		// lsof itself shows up when it inherits a working folder on the drive; it's gone on retry.
		if name == "lsof" {
			continue
		}
		if name == "" {
			name = "unknown process"
		}
		holders = append(holders, Holder{Name: name, PID: pid})
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
