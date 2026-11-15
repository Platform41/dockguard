package output

import (
	"bytes"
	"testing"
)

func TestFormatAllocatedSize(t *testing.T) {
	for _, tt := range []struct {
		bytes int64
		want  string
	}{{0, "0 B"}, {512, "512 B"}, {1000, "1 KB"}, {1200, "1.2 KB"}, {12000, "12 KB"}, {268435456, "268 MB"}, {1200000000, "1.2 GB"}} {
		if got := FormatAllocatedSize(tt.bytes); got != tt.want {
			t.Fatalf("%d: %s", tt.bytes, got)
		}
	}
}

func TestVolumeSubrowAndInfo(t *testing.T) {
	old := Out
	defer func() { Out = old }()
	var buf bytes.Buffer
	Out = &buf
	PrintList([]Row{{Name: "fizzy", Kind: "container", State: "stopped", Path: "/data"}, {Name: "db", Kind: "volume", State: "missing", Path: "/data/volumes/db", Subrow: true}})
	want := "\nNAME    KIND       STATE    PATH\nfizzy   container  stopped  /data\n  └ db  volume     missing  /data/volumes/db\n"
	if buf.String() != want {
		t.Fatalf("table:\n%q", buf.String())
	}
	buf.Reset()
	PrintVolumeInfo("db", "missing", "/data/volumes/db")
	if buf.String() != "  volume db: missing (/data/volumes/db)\n" {
		t.Fatal(buf.String())
	}
}
