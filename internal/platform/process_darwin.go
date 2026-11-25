package platform

/*
#include <libproc.h>
#include <sys/proc.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

func processInfo(pid int) (C.struct_proc_bsdinfo, error) {
	var info C.struct_proc_bsdinfo
	size := C.int(unsafe.Sizeof(info))
	n, err := C.proc_pidinfo(C.int(pid), C.PROC_PIDTBSDINFO, 0, unsafe.Pointer(&info), size)
	if n != size {
		return info, processInspectionError(pid, err)
	}
	if info.pbi_status == C.SZOMB {
		return info, os.ErrProcessDone
	}
	return info, nil
}

func processParent(pid int) (int, error) {
	info, err := processInfo(pid)
	return int(info.pbi_ppid), err
}

func inspectProcess(pid int) (Process, error) {
	info, err := processInfo(pid)
	if err != nil {
		return Process{}, err
	}
	var path [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	n, err := C.proc_pidpath(C.int(pid), unsafe.Pointer(&path[0]), C.uint32_t(len(path)))
	if n <= 0 {
		return Process{}, processInspectionError(pid, err)
	}
	// The path and BSD metadata are separate kernel calls. Check that the
	// process did not change identity between them.
	current, err := processInfo(pid)
	if err != nil {
		return Process{}, err
	}
	name := processName(info)
	if info.pbi_start_tvsec != current.pbi_start_tvsec || info.pbi_start_tvusec != current.pbi_start_tvusec ||
		info.pbi_uid != current.pbi_uid || name != processName(current) {
		return Process{}, os.ErrProcessDone
	}
	return Process{
		Name: name, Path: C.GoString(&path[0]),
		Started: time.Unix(int64(info.pbi_start_tvsec), int64(info.pbi_start_tvusec)*1000),
		UID:     int(info.pbi_uid), PPID: int(current.pbi_ppid),
	}, nil
}

func processName(info C.struct_proc_bsdinfo) string {
	if name := C.GoString(&info.pbi_name[0]); name != "" {
		return name
	}
	return C.GoString(&info.pbi_comm[0])
}

func processInspectionError(pid int, err error) error {
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	if err == nil {
		return fmt.Errorf("inspect PID %d: incomplete process information", pid)
	}
	return fmt.Errorf("inspect PID %d: %w", pid, err)
}
