package app

import (
	"fmt"

	"github.com/platform41/dockguard/internal/output"
	containerkind "github.com/platform41/dockguard/internal/target/container"
)

type volumeLister interface {
	Volumes() []containerkind.VolumeStatus
}

var _ volumeLister = (*containerkind.Target)(nil)

func volumeStatuses(e entry, mounted bool) []containerkind.VolumeStatus {
	if e.cfg.Container == nil || len(e.cfg.Container.Volumes) == 0 {
		return nil
	}
	if !mounted {
		volumes := make([]containerkind.VolumeStatus, 0, len(e.cfg.Container.Volumes))
		for _, name := range e.cfg.Container.Volumes {
			volumes = append(volumes, containerkind.VolumeStatus{Name: name, Path: containerkind.VolumePath(e.cfg.Container.DataPath, name), State: "drive not mounted"})
		}
		return volumes
	}
	if target, ok := e.t.(volumeLister); ok {
		return target.Volumes()
	}
	return nil
}

func volumeLabel(v containerkind.VolumeStatus) string {
	if v.Err != nil {
		return fmt.Sprintf("unknown (%v)", v.Err)
	}
	if v.State == "present" {
		return "present, " + output.FormatAllocatedSize(v.AllocatedBytes)
	}
	return v.State
}
