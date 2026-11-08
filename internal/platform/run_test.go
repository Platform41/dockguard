package platform

import (
	"strings"
	"testing"
	"time"
)

// Review on PR #7: a child that keeps the output open must not make RunDetached wait for it.
func TestRunDetachedDoesNotWaitForInheritedOutput(t *testing.T) {
	start := time.Now()
	out, code, err := RunDetached("/bin/sh", "-c", "sleep 3 & echo started")
	if err != nil || code != 0 || !strings.Contains(out, "started") {
		t.Fatalf("RunDetached = %q, %d, %v", out, code, err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("RunDetached took %v; it waited for the background child", took)
	}
}

func TestRunReportsExitCode(t *testing.T) {
	_, code, err := Run("/bin/sh", "-c", "exit 3")
	if err != nil || code != 3 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	if _, _, err := Run("/nonexistent/binary"); err == nil {
		t.Fatal("Run(missing binary) error = nil")
	}
}
