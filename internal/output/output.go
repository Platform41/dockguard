package output

import (
	"fmt"

	"github.com/ningenai/dockguard/internal/checks"
)

func PrintUsage() {
	fmt.Println("DockGuard")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  dockguard status")
	fmt.Println("  dockguard check")
	fmt.Println("  dockguard start")
}

func PrintStatus(status checks.Status) {
	fmt.Printf("Status: %s\n", status.Summary)
	for _, item := range status.Items {
		fmt.Printf("- %s: %s\n", formatState(item.OK), item.Name)
	}
}

func PrintCheckResult(result checks.Result) {
	for _, item := range result.Items {
		fmt.Printf("- %s: %s", formatState(item.OK), item.Name)
		if item.Message != "" {
			fmt.Printf(" (%s)", item.Message)
		}
		fmt.Println()
	}
}

func PrintStarted() {
	fmt.Println("Docker Desktop start requested.")
}

func formatState(ok bool) string {
	if ok {
		return "ok"
	}
	return "fail"
}
