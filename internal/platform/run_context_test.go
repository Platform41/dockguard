package platform

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunDetachedContextReportsDeadlineAndKillsProcessGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	out, code, err := RunDetachedContext(ctx, "/bin/sh", "-c", "sleep 30 & echo $!; wait")
	if !errors.Is(err, context.DeadlineExceeded) || code != -1 {
		t.Fatalf("RunDetachedContext = %q, %d, %v; want deadline error", out, code, err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("RunDetachedContext took %v; it waited for a descendant", took)
	}

	pid, parseErr := strconv.Atoi(strings.TrimSpace(out))
	if parseErr != nil {
		t.Fatalf("child PID in output %q: %v", out, parseErr)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant PID %d survived the attempt deadline: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunDetachedContextCapturesOutputAndReportsNonZeroExit(t *testing.T) {
	out, code, err := RunDetachedContext(context.Background(), "/bin/sh", "-c", "echo not-ready; exit 3")
	if err != nil || code != 3 || !strings.Contains(out, "not-ready") {
		t.Fatalf("RunDetachedContext = %q, %d, %v", out, code, err)
	}
}
