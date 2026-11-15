package platform

import (
	"errors"
	"syscall"

	"github.com/platform41/dockguard/internal/pathutil"
)

var volumeStatfs = syscall.Statfs

// IsMountPoint distinguishes a mount from an ordinary directory locally.
// diskutil info returns an error for ordinary directories, so it cannot make
// that distinction without confusing an inspection failure with a stray folder.
func IsMountPoint(path string) (bool, error) {
	resolved := pathutil.Resolve(path)
	var st syscall.Statfs_t
	if err := volumeStatfs(resolved, &st); err != nil {
		return false, err
	}
	var mount []byte
	for _, ch := range st.Mntonname {
		if ch == 0 {
			break
		}
		mount = append(mount, byte(ch))
	}
	if len(mount) == 0 {
		return false, errors.New("filesystem inspection returned no mount point")
	}
	return pathutil.Resolve(string(mount)) == resolved, nil
}
