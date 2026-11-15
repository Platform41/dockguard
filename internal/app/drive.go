package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/platform41/dockguard/internal/config"
	"github.com/platform41/dockguard/internal/drive"
	"github.com/platform41/dockguard/internal/output"
	"github.com/platform41/dockguard/internal/pathutil"
	"github.com/platform41/dockguard/internal/platform"
)

var (
	inspectDrive = drive.Inspect
	volumeAt     = platform.VolumeAt
	mountVolume  = platform.MountVolume
)

func driveStatus(cfg config.Config) drive.Status {
	if cfg.DriveUUID != "" {
		return inspectDrive(cfg.DriveUUID, cfg.Drive)
	}
	state := drive.NotMounted
	if driveMounted(cfg.Drive) {
		state = drive.Mounted
	}
	return drive.Status{State: state, Message: string(state)}
}

func printDriveHeader(cfg config.Config, s drive.Status) {
	if cfg.DriveUUID == "" {
		output.PrintHeader(cfg.Path, cfg.Drive, s.Mounted())
		fmt.Fprintln(output.Out, "Drive not registered; run dockguard drive.")
		return
	}
	fmt.Fprintf(output.Out, "Config: %s\nDrive:  %s (%s)\n", cfg.Path, cfg.Drive, s.State)
	if !s.Mounted() {
		fmt.Fprintln(output.Out, s.Message)
	}
}

func runDrive(cfg config.Config) (int, error) {
	v, err := volumeAt(cfg.Drive)
	if err != nil {
		return 1, fmt.Errorf("inspect drive %s: %w", cfg.Drive, err)
	}
	if v.MountPoint == "" || pathutil.Resolve(v.MountPoint) != pathutil.Resolve(cfg.Drive) {
		return 1, fmt.Errorf("%s is not a mount point", cfg.Drive)
	}
	location := "external"
	if v.Internal {
		location = "internal"
	}
	fmt.Fprintf(output.Out, "Drive  %s\nVolume %s (%s, %s)\nUUID   %s\n", cfg.Drive, v.Name, v.Filesystem, location, v.UUID)
	if cfg.DriveUUID != "" {
		s := driveStatus(cfg)
		fmt.Fprintf(output.Out, "Status %s\n%s\n", s.State, s.Message)
		if strings.EqualFold(v.UUID, cfg.DriveUUID) {
			return 0, nil
		}
	} else {
		fmt.Fprintln(output.Out, "Status not registered")
	}
	fmt.Fprintf(output.Out, "\nTo register this drive, add to dockguard.yaml:\ndrive_uuid: %s\n", v.UUID)
	return 0, nil
}

func runMount(cfg config.Config) (int, error) {
	if cfg.DriveUUID == "" {
		return 1, errors.New("No drive registered; run dockguard drive.")
	}
	before := driveStatus(cfg)
	name := before.Volume.Name
	if name == "" {
		name = filepath.Base(cfg.Drive)
	}
	switch before.State {
	case drive.Mounted:
		fmt.Fprintf(output.Out, "%s is already mounted at %s.\n", name, cfg.Drive)
		return 0, nil
	case drive.NotMounted:
	case drive.NotConnected:
		return 1, fmt.Errorf("%s is not connected; plug it in.", name)
	default:
		return 1, fmt.Errorf("%s: %s", before.State, before.Message)
	}
	fmt.Fprintf(output.Out, "Mounting %s…\n", name)
	err := mountVolume(context.Background(), cfg.DriveUUID)
	after := driveStatus(cfg)
	if errors.Is(err, context.DeadlineExceeded) {
		return 1, fmt.Errorf("%s didn't mount within 90 s; drive status: %s (%s).", name, after.State, after.Message)
	}
	if err != nil {
		return 1, fmt.Errorf("%w; drive status: %s (%s)", err, after.State, after.Message)
	}
	if !after.Mounted() {
		return 1, fmt.Errorf("%s: %s", after.State, after.Message)
	}
	fmt.Fprintf(output.Out, "%s mounted at %s.\n", name, cfg.Drive)
	return 0, nil
}
