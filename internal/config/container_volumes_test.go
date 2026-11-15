package config

import (
	"strings"
	"testing"
)

// AS-85: only named, local container volumes with a data_path are accepted.
func TestContainerVolumesConfig(t *testing.T) {
	const base = "drive: /Volumes/ExternalSSD\ntargets:\n  fizzy:\n    kind: container\n    up: /scripts/up\n    down: /scripts/down\n    containers: [db]\n"
	for _, tt := range []struct {
		name, extra string
		valid       bool
	}{
		{"valid", "    data_path: /Volumes/ExternalSSD/data\n    volumes: [db, DB_2.0-backup]\n", true},
		{"empty list", "    data_path: /Volumes/ExternalSSD/data\n    volumes: []\n", true},
		{"missing data path", "    volumes: [db]\n", false},
		{"empty list missing data path", "    volumes: []\n", false},
		{"empty name", "    data_path: /data\n    volumes: ['']\n", false},
		{"traversal", "    data_path: /data\n    volumes: [../db]\n", false},
		{"double dot", "    data_path: /data\n    volumes: [db..backup]\n", false},
		{"slash", "    data_path: /data\n    volumes: [db/backup]\n", false},
		{"leading dash", "    data_path: /data\n    volumes: [-db]\n", false},
		{"spaces", "    data_path: /data\n    volumes: [' db ']\n", false},
		{"duplicate", "    data_path: /data\n    volumes: [db, db]\n", false},
		{"scalar", "    data_path: /data\n    volumes: db\n", false},
		{"null", "    data_path: /data\n    volumes: null\n", false},
		{"number", "    data_path: /data\n    volumes: [123]\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse([]byte(base + tt.extra))
			if tt.valid {
				if err != nil {
					t.Fatal(err)
				}
				if tt.name == "valid" && strings.Join(cfg.Targets[0].Container.Volumes, ",") != "db,DB_2.0-backup" {
					t.Fatalf("order %+v", cfg)
				}
			} else if err == nil || !strings.Contains(err.Error(), "fizzy") || !strings.Contains(err.Error(), "volumes") {
				t.Fatalf("error %v", err)
			}
		})
	}
	for _, kind := range []string{"app", "postgres", "docker-desktop"} {
		_, err := Parse([]byte("drive: /Volumes/ExternalSSD\ntargets:\n  other:\n    kind: " + kind + "\n    volumes: [db]\n"))
		if err == nil || !strings.Contains(err.Error(), "volumes") || !strings.Contains(err.Error(), "other") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}
