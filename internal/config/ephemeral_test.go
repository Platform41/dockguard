package config

import (
	"strings"
	"testing"
)

func TestContainerEphemeralConfig(t *testing.T) {
	cfg, err := parseReadinessConfig(t, "    ephemeral: true")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Targets[0].Container.Ephemeral {
		t.Fatal("ephemeral: true was not read")
	}
}

func TestContainerEphemeralDefaultsFalse(t *testing.T) {
	cfg, err := parseReadinessConfig(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Targets[0].Container.Ephemeral {
		t.Fatal("ephemeral defaulted to true")
	}
}

func TestOtherKindsRejectEphemeral(t *testing.T) {
	_, err := Parse([]byte(`
drive: /Volumes/X
targets:
  xcode:
    kind: app
    app: Xcode
    ephemeral: true
`))
	if err == nil || !strings.Contains(err.Error(), `unknown key "ephemeral"`) {
		t.Fatalf("app with ephemeral: error = %v", err)
	}
}
