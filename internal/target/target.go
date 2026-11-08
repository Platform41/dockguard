// Package target defines a managed application on the external drive
// (docs/specs/multi-target.md §1, §2.2 and §6).
package target

import "github.com/platform41/dockguard/internal/checks"

// State is what a probe confirmed about a target.
type State int

const (
	// Unknown means the probe failed or its answer couldn't be read.
	Unknown State = iota
	// Stopped means nothing of the target is running.
	Stopped
	// Running means the target is fully running.
	Running
	// Partial means some of the target is running and some isn't.
	Partial
)

func (s State) String() string {
	switch s {
	case Stopped:
		return "stopped"
	case Running:
		return "running"
	case Partial:
		return "partial"
	default:
		return "unknown"
	}
}

// Target is one managed application.
type Target interface {
	Name() string
	Kind() string
	// MainPath is the target's main path on the drive, shown by list.
	MainPath() string
	// Checks are the preflight checks, without the drive check or the already-running guard.
	Checks() []checks.Item
	// State probes the target. The error explains Unknown.
	State() (State, error)
	Start() error
	// Stop reports false when the target was already stopped.
	Stop() (bool, error)
}
