package config

import (
	"strings"
	"testing"
)

func parseReadinessConfig(t *testing.T, extra string) (Config, error) {
	t.Helper()
	return Parse([]byte(strings.TrimSpace(`
drive: /Volumes/ExternalSSD
targets:
  fizzy:
    kind: container
    up: /x/up.sh
    down: /x/down.sh
    containers: [fizzy-app]
` + "\n" + extra)))
}

func TestContainerReadinessConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := parseReadinessConfig(t, "    ready: ~/bin/ready.sh\n    ready_timeout_seconds: 12")
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Targets[0].Container
	if c.Ready != home+"/bin/ready.sh" || c.ReadyTimeoutSeconds != 12 {
		t.Fatalf("container readiness = %#v", c)
	}
}

// AS-50: omitting ready preserves the existing start path; the timeout still has its default.
func TestContainerReadinessDefaults(t *testing.T) {
	cfg, err := parseReadinessConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Targets[0].Container
	if c.Ready != "" || c.ReadyTimeoutSeconds != 30 {
		t.Fatalf("container readiness defaults = %#v", c)
	}
}

// AS-51: invalid timeouts name both the target and the key.
func TestContainerReadinessTimeoutRange(t *testing.T) {
	for _, value := range []string{"0", "-1", "301"} {
		_, err := parseReadinessConfig(t, "    ready: /x/ready.sh\n    ready_timeout_seconds: "+value)
		if err == nil || !strings.Contains(err.Error(), `target "fizzy"`) ||
			!strings.Contains(err.Error(), "ready_timeout_seconds") {
			t.Fatalf("timeout %s: error = %v", value, err)
		}
	}
}

func TestContainerReadyMustBeAbsolute(t *testing.T) {
	_, err := parseReadinessConfig(t, "    ready: ready.sh")
	if err == nil || !strings.Contains(err.Error(), "ready must be an absolute path") {
		t.Fatalf("relative ready: error = %v", err)
	}
}
