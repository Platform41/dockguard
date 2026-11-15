// Package version reports DockGuard build versions and manages release updates.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	modulePath  = "github.com/platform41/dockguard"
	commandPath = modulePath + "/cmd/dockguard"
)

var readBuildInfo = debug.ReadBuildInfo

// Info describes a DockGuard build.
type Info struct {
	Version     string
	GoVersion   string
	Platform    string
	Commit      string
	Modified    bool
	Development bool
}

// Current reads the running binary's build information.
func Current() Info {
	build, ok := readBuildInfo()
	if !ok || build == nil || build.Main.Version == "" || build.Main.Version == "(devel)" {
		return Info{}
	}
	info := Info{
		Version:     build.Main.Version,
		GoVersion:   build.GoVersion,
		Platform:    runtimePlatform(),
		Development: isPseudoVersion(build.Main.Version),
	}
	for _, setting := range build.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Commit = setting.Value
		case "vcs.modified":
			info.Modified = setting.Value == "true"
		}
	}
	if info.Commit == "" && info.Development {
		parts := strings.Split(info.Version, "-")
		info.Commit = parts[len(parts)-1]
	}
	return info
}

func runtimePlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// String formats the version line printed by dockguard version.
func (i Info) String() string {
	if i.Version == "" {
		return "DockGuard (version unknown)"
	}
	if i.Development {
		line := fmt.Sprintf("DockGuard %s (development build, commit %s", i.Version, i.Commit)
		if i.Modified {
			line += ", modified"
		}
		return line + ")"
	}
	return fmt.Sprintf("DockGuard %s (%s, %s)", i.Version, i.GoVersion, i.Platform)
}

// InstallCommand returns the pinned update command shown to users.
func InstallCommand(release string) string {
	return "go install " + commandPath + "@" + release
}
