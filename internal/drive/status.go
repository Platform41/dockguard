// Package drive verifies registered volume identity before storage is trusted.
package drive

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/platform41/dockguard/internal/pathutil"
	"github.com/platform41/dockguard/internal/platform"
)

type State string

const (
	Mounted          State = "mounted"
	NotMounted       State = "not mounted"
	NotConnected     State = "not connected"
	MountedElsewhere State = "mounted elsewhere"
	WrongVolume      State = "wrong volume"
	StrayFolder      State = "stray folder"
	Unknown          State = "unknown"
)

type Status struct {
	State   State
	Volume  platform.Volume
	Message string
}

func (s Status) Mounted() bool { return s.State == Mounted }

// Inspector separates OS discovery from the status rules for fake-based tests.
type Inspector struct {
	FindVolume   func(string) (platform.Volume, error)
	VolumeAt     func(string) (platform.Volume, error)
	PathExists   func(string) error
	IsMountPoint func(string) (bool, error)
}

func Inspect(uuid, path string) Status {
	return (Inspector{FindVolume: platform.FindVolume, VolumeAt: platform.VolumeAt, IsMountPoint: platform.IsMountPoint, PathExists: func(path string) error { _, err := os.Stat(path); return err }}).Status(uuid, path)
}

func (i Inspector) Status(uuid, path string) Status {
	volume, err := i.FindVolume(uuid)
	result := Status{Volume: volume}
	set := func(state State, message string) Status {
		result.State = state
		result.Message = message
		return result
	}
	if errors.Is(err, platform.ErrVolumeNotFound) {
		return set(NotConnected, "registered volume "+uuid+" is not connected; plug it in")
	}
	if err != nil {
		return set(Unknown, err.Error())
	}
	if !strings.EqualFold(volume.UUID, uuid) {
		return set(Unknown, "volume lookup returned a different UUID")
	}
	// Resolve existing aliases without collapsing symlink/.. paths lexically.
	expected := pathutil.Resolve(path)
	if volume.MountPoint != "" {
		if pathutil.Resolve(volume.MountPoint) == expected {
			return set(Mounted, "registered volume "+uuid+" mounted at "+path)
		}
		return set(MountedElsewhere, fmt.Sprintf("registered volume mounted at %s, not %s", volume.MountPoint, path))
	}
	err = i.PathExists(path)
	if errors.Is(err, os.ErrNotExist) {
		return set(NotMounted, "registered volume is attached but not mounted; run dockguard mount")
	}
	if err != nil {
		return set(Unknown, err.Error())
	}
	mounted, err := i.IsMountPoint(path)
	if err != nil {
		return set(Unknown, err.Error())
	}
	if !mounted {
		return set(StrayFolder, path+" exists but is not a mount point")
	}
	at, err := i.VolumeAt(path)
	if err != nil {
		return set(Unknown, err.Error())
	}
	if at.MountPoint == "" || pathutil.Resolve(at.MountPoint) != expected {
		return set(StrayFolder, path+" exists but is not a mount point")
	}
	if !strings.EqualFold(at.UUID, uuid) {
		return set(WrongVolume, fmt.Sprintf("wrong volume at %s: UUID %s, registered UUID %s", path, at.UUID, uuid))
	}
	// The volume changed between observations. A later snapshot can confirm it.
	return set(Unknown, "volume changed during inspection; retry")
}
