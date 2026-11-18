// Package containerstate shares Apple container status and shutdown safety probes.
package containerstate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Snapshot is one probe of the container system.
type Snapshot struct {
	SystemUp bool
	// Running holds every running container on the machine, by name.
	Running map[string]bool
}

// Probe reads the container system's status and, when it's up, every container's state.
//
// Only a confirmed answer counts. The system is down only when its status is "unregistered": no
// launchd job exists, so nothing can be running. A registered daemon that fails its health check
// reports something else, and that stays unknown, which blocks eject.
func Probe(run func(string, ...string) (string, int, error)) (Snapshot, error) {
	output, code, err := run("container", "system", "status", "--format", "json")
	if err != nil {
		return Snapshot{}, fmt.Errorf("container system status: %w", err)
	}
	var status struct {
		Status string `json:"status"`
	}
	if jsonErr := json.Unmarshal([]byte(output), &status); jsonErr != nil {
		return Snapshot{}, fmt.Errorf("container system status: exit %d, unreadable output: %s", code, strings.TrimSpace(output))
	}
	switch {
	case status.Status == "unregistered":
		return Snapshot{SystemUp: false}, nil
	case status.Status == "running" && code == 0:
	default:
		return Snapshot{}, fmt.Errorf("container system status: exit %d, status %q", code, status.Status)
	}

	output, code, err = run("container", "list", "--all", "--format", "json")
	if err != nil || code != 0 {
		return Snapshot{}, fmt.Errorf("container list: exit %d: %s %v", code, strings.TrimSpace(output), errOrEmpty(err))
	}
	// Pointers tell a missing field from an empty one: an entry without an id or a state can't be
	// matched to the config, so the whole list is treated as unreadable rather than guessed at.
	var list []struct {
		ID     *string `json:"id"`
		Status *struct {
			State *string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return Snapshot{}, fmt.Errorf("container list: can't read its JSON: %w", err)
	}
	// "[]" decodes to an empty list (no containers); "null" leaves it nil, which is no answer.
	if list == nil {
		return Snapshot{}, fmt.Errorf("container list: got %q instead of a list", strings.TrimSpace(output))
	}

	snap := Snapshot{SystemUp: true, Running: map[string]bool{}}
	for i, c := range list {
		if c.ID == nil || *c.ID == "" || c.Status == nil || c.Status.State == nil || *c.Status.State == "" {
			return Snapshot{}, fmt.Errorf("container list: entry %d has no id or state", i)
		}
		// Anything other than "stopped" (running, starting, stopping...) may be using the drive.
		if *c.Status.State != "stopped" {
			snap.Running[*c.ID] = true
		}
	}
	return snap, nil
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// OtherRunning returns sorted names of active containers outside the managed set.
func OtherRunning(snap Snapshot, managed []string) []string {
	mine := map[string]bool{}
	for _, id := range managed {
		mine[id] = true
	}
	var others []string
	for id := range snap.Running {
		if !mine[id] {
			others = append(others, id)
		}
	}
	sort.Strings(others)
	return others
}
