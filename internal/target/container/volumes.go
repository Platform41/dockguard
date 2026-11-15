package container

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"syscall"
)

var volumeStat = os.Stat

// VolumeStatus describes persistent storage independently of the stack lifecycle.
type VolumeStatus struct {
	Name, Path, State string
	AllocatedBytes    int64
	Err               error
}

// VolumePath preserves data_path's symlink/.. meaning instead of cleaning it.
func VolumePath(dataPath, name string) string {
	return strings.TrimRight(dataPath, "/") + "/volumes/" + name
}

// Volumes only stats configured files; it never probes the container system.
func (t *Target) Volumes() []VolumeStatus {
	result := make([]VolumeStatus, 0, len(t.cfg.Container.Volumes))
	for _, name := range t.cfg.Container.Volumes {
		v := VolumeStatus{Name: name, Path: VolumePath(t.cfg.Container.DataPath, name)}
		info, err := volumeStat(v.Path + "/volume.img")
		switch {
		case errors.Is(err, os.ErrNotExist):
			v.State = "missing"
		case err != nil:
			v.State = "unknown"
			v.Err = err
		case !info.Mode().IsRegular():
			v.State = "unknown"
			v.Err = fmt.Errorf("volume.img is not a regular file")
		default:
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok || st == nil || st.Blocks < 0 || st.Blocks > math.MaxInt64/512 {
				v.State = "unknown"
				v.Err = fmt.Errorf("allocated size unavailable")
			} else {
				v.State = "present"
				v.AllocatedBytes = st.Blocks * 512
			}
		}
		result = append(result, v)
	}
	return result
}
