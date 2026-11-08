// Package output prints DockGuard's plain-text output.
package output

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/platform41/dockguard/internal/checks"
)

// Out is where output goes. Tests replace it.
var Out io.Writer = os.Stdout

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
)

func PrintUsage() {
	fmt.Fprintln(Out, "DockGuard")
	fmt.Fprintln(Out, "")
	fmt.Fprintln(Out, "Usage:")
	fmt.Fprintln(Out, "  dockguard list [--config path]")
	fmt.Fprintln(Out, "  dockguard status [--config path]")
	fmt.Fprintln(Out, "  dockguard check [target] [--config path]")
	fmt.Fprintln(Out, "  dockguard start [target] [--config path]")
	fmt.Fprintln(Out, "  dockguard stop [target] [--config path] [--eject]")
	fmt.Fprintln(Out, "")
	fmt.Fprintln(Out, "With no target, check, start and stop act on the target named \"docker\".")
	fmt.Fprintln(Out, "Config lookup: --config, then ./dockguard.yaml, then ~/.config/dockguard/dockguard.yaml.")
}

// PrintHeader prints the config file used and the drive's state.
func PrintHeader(configPath, drive string, mounted bool) {
	state := "mounted"
	if !mounted {
		state = "not mounted"
	}
	fmt.Fprintf(Out, "Config: %s\n", configPath)
	fmt.Fprintf(Out, "Drive:  %s (%s)\n", drive, state)
}

// PrintLegacyNotice suggests the current config format (AS-02).
func PrintLegacyNotice() {
	fmt.Fprintln(Out, "Note: this config uses the v0.1 format. See examples/dockguard.yaml for the current format.")
}

// Row is one line of dockguard list.
type Row struct {
	Name, Kind, State, Path string
}

// PrintList prints the target table.
func PrintList(rows []Row) {
	fmt.Fprintln(Out)
	w := tabwriter.NewWriter(Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tKIND\tSTATE\tPATH")
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Kind, r.State, r.Path)
	}
	_ = w.Flush()
}

// PrintTargetHeading starts a target's section in status.
func PrintTargetHeading(name, kind, state string) {
	fmt.Fprintf(Out, "\n%s (%s): %s\n", name, kind, state)
}

// PrintItems prints one line per check.
func PrintItems(items []checks.Item) {
	for _, item := range items {
		fmt.Fprintf(Out, "- %s %s", formatState(item), item.Name)
		if item.Message != "" {
			fmt.Fprintf(Out, " (%s)", item.Message)
		}
		fmt.Fprintln(Out)
	}
}

// PrintSummary prints the overall status line.
func PrintSummary(ready bool) {
	summary := "ready"
	if !ready {
		summary = "not ready"
	}
	fmt.Fprintf(Out, "\nStatus: %s\n", summary)
}

// Blocker is a target that prevents an eject.
type Blocker struct {
	Name, State, Reason string
}

// PrintEjectRefused explains why the drive wasn't ejected (spec §2.3).
func PrintEjectRefused(drive string, blockers []Blocker) {
	fmt.Fprintf(Out, "Not ejecting %s: these targets aren't confirmed stopped:\n", drive)
	for _, b := range blockers {
		fmt.Fprintf(Out, "- %s: %s", b.Name, b.State)
		if b.Reason != "" {
			fmt.Fprintf(Out, " (%s)", b.Reason)
		}
		fmt.Fprintln(Out)
	}
}

func PrintStarted(display string) {
	fmt.Fprintf(Out, "%s start requested.\n", display)
}

func PrintStopped(display string) {
	fmt.Fprintf(Out, "%s stop requested.\n", display)
}

func PrintAlreadyStopped(display string) {
	fmt.Fprintf(Out, "%s is already stopped.\n", display)
}

func PrintEjected(path string) {
	fmt.Fprintf(Out, "External volume ejected: %s\n", path)
}

func formatState(item checks.Item) string {
	switch {
	case item.Skipped:
		return colorize("[skip]", colorYellow)
	case item.OK:
		return colorize("[ok]", colorGreen)
	default:
		return colorize("[fail]", colorRed)
	}
}

func colorize(text, color string) string {
	if !shouldUseColor() {
		return text
	}

	return color + text + colorReset
}

func shouldUseColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	file, ok := Out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}
