//go:build !cgo

package platform

import "errors"

var errProcessInspection = errors.New("process inspection requires a build with cgo enabled")

func processParent(int) (int, error)      { return 0, errProcessInspection }
func inspectProcess(int) (Process, error) { return Process{}, errProcessInspection }
