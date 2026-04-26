package output

import (
	"fmt"
	"os"

	"github.com/platform41/dockguard/internal/checks"
)

const (
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
)

func PrintUsage() {
	fmt.Println("DockGuard")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  dockguard status [--config path]")
	fmt.Println("  dockguard check [--config path]")
	fmt.Println("  dockguard start [--config path]")
	fmt.Println("  dockguard stop [--config path] [--eject]")
}

func PrintStatus(status checks.Status) {
	fmt.Printf("Status: %s\n", status.Summary)
	for _, item := range status.Items {
		fmt.Printf("- %s %s", formatState(item.OK), item.Name)
		if item.Message != "" {
			fmt.Printf(" (%s)", item.Message)
		}
		fmt.Println()
	}
}

func PrintCheckResult(result checks.Result) {
	for _, item := range result.Items {
		fmt.Printf("- %s %s", formatState(item.OK), item.Name)
		if item.Message != "" {
			fmt.Printf(" (%s)", item.Message)
		}
		fmt.Println()
	}
}

func PrintStarted() {
	fmt.Println("Docker Desktop start requested.")
}

func PrintStopped() {
	fmt.Println("Docker Desktop stop requested.")
}

func PrintAlreadyStopped() {
	fmt.Println("Docker Desktop is already stopped.")
}

func PrintEjected(path string) {
	fmt.Printf("External volume ejected: %s\n", path)
}

func formatState(ok bool) string {
	if ok {
		return colorize("[ok]", colorGreen)
	}
	return colorize("[fail]", colorRed)
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

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}
