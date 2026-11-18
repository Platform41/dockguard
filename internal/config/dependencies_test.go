package config

import (
	"strings"
	"testing"
)

// AS-105: reject invalid references before any target operation.
func TestDependencyValidation(t *testing.T) {
	for _, tt := range []struct{ name, deps, other, want string }{
		{"unknown", "[missing]", "", `target "myapp"`},
		{"self", "[myapp]", "", "itself"},
		{"duplicate", "[base, base]", "  base: {kind: app, app: Base}\n", "twice"},
		{"cycle", "[base]", "  base: {kind: app, app: Base, depends_on: [myapp]}\n", "myapp → base → myapp"},
		{"not list", "base", "", "list"},
		{"not string", "[42]", "", "strings"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte("drive: /Volumes/ExternalSSD\ntargets:\n  myapp: {kind: app, app: MyApp, depends_on: " + tt.deps + "}\n" + tt.other))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want %s", err, tt.want)
			}
		})
	}
}

func TestDependenciesAllowedOnEveryKind(t *testing.T) {
	cfg, err := Parse([]byte(`drive: /Volumes/ExternalSSD
targets:
  system: {kind: container-system, data_path: /Volumes/ExternalSSD/container-data}
  myapp: {kind: app, app: MyApp, depends_on: [system]}
  db: {kind: postgres, data_dir: /Volumes/ExternalSSD/db, bin_dir: /opt/postgres/bin, depends_on: [system]}
  stack: {kind: container, up: /scripts/up, down: /scripts/down, containers: [myapp], depends_on: [system]}
  docker: {kind: docker-desktop, storage_path: /Volumes/ExternalSSD/docker, settings_path: /settings.json, depends_on: [system]}
  other-system: {kind: container-system, depends_on: [system]}
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range cfg.Targets[1:] {
		if len(item.DependsOn) != 1 || item.DependsOn[0] != "system" {
			t.Fatalf("%s: %v", item.Name, item.DependsOn)
		}
	}
	if cfg.Targets[0].ContainerSystem.DataPath != "/Volumes/ExternalSSD/container-data" {
		t.Fatal("data_path lost")
	}
}

// AS-120: a legacy system stop must not overlap the explicit system target.
func TestContainerSystemConflictsWithStopSystem(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		_, err := Parse([]byte(`drive: /Volumes/ExternalSSD
targets:
  stack: {kind: container, up: /scripts/up, down: /scripts/down, containers: [myapp], stop_system: ` + value + `}
  system: {kind: container-system}
`))
		if value == "false" {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), `target "stack"`) || !strings.Contains(err.Error(), "stop_system") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestContainerSystemRejectsStackKeysAndRelativePath(t *testing.T) {
	for _, extra := range []string{"up: /scripts/up", "stop_system: true", "data_path: relative", "containers: [myapp]"} {
		if _, err := Parse([]byte("drive: /Volumes/ExternalSSD\ntargets:\n  system: {kind: container-system, " + extra + "}\n")); err == nil {
			t.Fatalf("accepted %s", extra)
		}
	}
}
