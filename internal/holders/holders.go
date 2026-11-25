// Package holders classifies processes holding a volume and stops them with confirmation.
package holders

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/platform41/dockguard/internal/platform"
)

type Class string

const (
	Stoppable Class = "stoppable"
	OtherUser Class = "other user"
	Self      Class = "self"
	Protected Class = "protected"
)

type Status string

const (
	Stopped      Status = "stopped"
	ForceStopped Status = "force-stopped"
	Declined     Status = "declined"
	Gone         Status = "gone"
	Failed       Status = "failed"
	NotOffered   Status = "not offered"
)

type Candidate struct {
	platform.Holder
	Class Class
	Hint  string

	// A known class may still lack a verified identity (for example, a process
	// that exits before inspection). A candidate without one cannot be offered.
	process platform.Process
}

func (c Candidate) String() string {
	label := fmt.Sprintf("%s [%s]", c.Holder, c.Class)
	if c.Hint != "" {
		label += " — " + c.Hint
	}
	return label
}

func (c Candidate) CanStop() bool {
	return c.Class == Stoppable && !c.process.Started.IsZero()
}

type Result struct {
	Status  Status
	Message string
	Err     error
}

type Prompt func(question string, defaultYes bool) bool

type Service struct {
	PID, UID  int
	Scan      func(string) ([]platform.Holder, error)
	Inspect   func(int) (platform.Process, error)
	Signal    func(int, syscall.Signal) error
	Ancestors func(int) ([]int, error)
	Now       func() time.Time
	Sleep     func(time.Duration)
}

func (s Service) Classify(list []platform.Holder) []Candidate {
	candidates := make([]Candidate, 0, len(list))
	if len(list) == 0 {
		return candidates
	}
	parents, parentErr := s.Ancestors(s.PID)
	for _, h := range list {
		c := Candidate{Holder: h}
		switch {
		case h.PID == s.PID || slices.Contains(parents, h.PID):
			c.Class, c.Hint = Self, "relaunch DockGuard from outside the drive"
		case h.UID < 0:
			c.Class, c.Hint = OtherUser, "owner unknown; DockGuard doesn't use sudo"
		case h.UID == 0 || h.UID != s.UID:
			user := h.User
			if user == "" {
				user = strconv.Itoa(h.UID)
			}
			c.Class, c.Hint = OtherUser, "owned by "+user+"; DockGuard doesn't use sudo"
		default:
			c.Class = Stoppable
			p, err := s.Inspect(h.PID)
			switch {
			case errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH):
				c.Hint = "already gone"
			case err != nil:
				c.Hint = "couldn't verify process: " + err.Error()
			case p.UID != h.UID:
				c.Hint = "already gone"
			default:
				switch hint := protectedHint(p.Path); {
				case hint != "":
					c.Class, c.Hint = Protected, hint
				case p.Name != h.Name:
					c.Hint = "already gone"
				case parentErr != nil:
					c.Hint = "couldn't verify DockGuard's ancestors: " + parentErr.Error()
				case !filepath.IsAbs(p.Path) || p.Started.IsZero():
					c.Hint = "couldn't verify process identity"
				default:
					c.process = p
				}
			}
		}
		candidates = append(candidates, c)
	}
	slices.SortFunc(candidates, func(a, b Candidate) int { return a.PID - b.PID })
	return candidates
}

func protectedHint(path string) string {
	path = filepath.Clean(path)
	if !strings.HasPrefix(path, "/System/") && !strings.HasPrefix(path, "/usr/") && !strings.HasPrefix(path, "/sbin/") {
		return ""
	}
	name := filepath.Base(path)
	switch {
	case name == "mds", name == "mds_stores", strings.HasPrefix(name, "mdworker"):
		return "Spotlight is indexing the drive; wait or exclude it in System Settings"
	case name == "launchd", name == "kernel_task", name == "WindowServer", name == "loginwindow",
		name == "Dock", name == "SystemUIServer", name == "fseventsd":
		return "protected macOS process; wait for macOS to release the drive"
	}
	return ""
}

func (s Service) Stop(mount string, c Candidate, ask Prompt) (Result, error) {
	if !c.CanStop() {
		return Result{Status: NotOffered}, nil
	}

	question := fmt.Sprintf("%s is using %s. Quit it?", c.Holder, mount)
	if c.process.Path == "/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder" {
		question = fmt.Sprintf("%s is using %s. macOS relaunches Finder automatically. Quit it?", c.Holder, mount)
	}
	if !ask(question, true) {
		return stillBlocking(), nil
	}
	if result, scanErr := s.verifyAndSignal(mount, c, syscall.SIGTERM); result.Status != "" || scanErr != nil {
		return result, scanErr
	}
	if result, scanErr := s.wait(mount, c.PID); result.Status != "" || scanErr != nil {
		return result, scanErr
	}
	if !ask(fmt.Sprintf("%s didn't quit. Force-quit it? Unsaved work will be lost.", c.Holder), false) {
		return stillBlocking(), nil
	}
	if result, scanErr := s.verifyAndSignal(mount, c, syscall.SIGKILL); result.Status != "" || scanErr != nil {
		return result, scanErr
	}

	deadline := s.Now().Add(stopWait)
	for {
		err := s.Signal(c.PID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return Result{Status: ForceStopped, Message: "force-quit"}, nil
		}
		p, inspectErr := s.Inspect(c.PID)
		if errors.Is(inspectErr, os.ErrProcessDone) || errors.Is(inspectErr, syscall.ESRCH) ||
			(inspectErr == nil && !p.Started.Equal(c.process.Started)) {
			return Result{Status: ForceStopped, Message: "force-quit"}, nil
		}
		if err != nil {
			return Result{Status: Failed, Err: err}, nil
		}
		if !s.Now().Before(deadline) {
			return Result{Status: Failed, Err: errors.New("process did not exit after SIGKILL")}, nil
		}
		s.Sleep(stopPoll)
	}
}

func stillBlocking() Result {
	return Result{Status: Declined, Message: "still using the drive"}
}

// An inspection immediately before signalling verifies the identity after the
// slower volume scan. The scan's record must also match the original owner and name.
func (s Service) verifyAndSignal(mount string, c Candidate, sig syscall.Signal) (Result, error) {
	list, scanErr := s.Scan(mount)
	if scanErr != nil {
		return Result{Status: Failed}, scanErr
	}
	h := findPID(list, c.PID)
	p, err := s.Inspect(c.PID)
	switch {
	case errors.Is(err, os.ErrProcessDone), errors.Is(err, syscall.ESRCH):
		return alreadyGone(), nil
	case h == nil:
		return Result{Status: Gone, Message: "no longer using the drive"}, nil
	case err != nil:
		return Result{Status: Failed, Err: err}, nil
	case h.UID != c.UID, h.Name != c.Name, !p.Started.Equal(c.process.Started), p.Name != c.process.Name,
		p.UID != c.process.UID, p.Path != c.process.Path:
		return alreadyGone(), nil
	}
	if err := s.Signal(c.PID, sig); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return alreadyGone(), nil
		}
		return Result{Status: Failed, Err: fmt.Errorf("%s: %w", sig, err)}, nil
	}
	return Result{}, nil
}

func alreadyGone() Result { return Result{Status: Gone, Message: "already gone"} }

func findPID(list []platform.Holder, pid int) *platform.Holder {
	for i := range list {
		if list[i].PID == pid {
			return &list[i]
		}
	}
	return nil
}

const (
	stopWait     = 5 * time.Second
	stopPoll     = 250 * time.Millisecond
	scanInterval = time.Second
)

func (s Service) wait(mount string, pid int) (Result, error) {
	start := s.Now()
	deadline, nextScan := start.Add(stopWait), start.Add(scanInterval)
	for {
		current := s.Now()
		if err := s.Signal(pid, 0); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return Result{Status: Stopped, Message: "quit"}, nil
			}
			// EPERM means the process still exists. A fresh scan will establish
			// whether it is still blocking, and re-verification guards SIGKILL.
			if !errors.Is(err, syscall.EPERM) {
				return Result{Status: Failed, Err: err}, nil
			}
		}
		atEnd := !current.Before(deadline)
		if atEnd || !current.Before(nextScan) {
			list, err := s.Scan(mount)
			if err != nil {
				return Result{Status: Failed}, err
			}
			if findPID(list, pid) == nil {
				return Result{Status: Stopped, Message: "released the drive"}, nil
			}
			if atEnd || !s.Now().Before(deadline) {
				return Result{}, nil
			}
			nextScan = current.Add(scanInterval)
		}
		s.Sleep(min(stopPoll, max(0, deadline.Sub(s.Now()))))
	}
}
