package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/platform41/dockguard/internal/target"
)

// AS-53: ephemeral is appended to dashboard state without breaking columns or narrow rows.
func TestEphemeralMarkerAndColumnsAS53(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running, target.Stopped)}
	f.snap.Rows[0].Ephemeral = true
	f.snap.Rows[0].Path = "/p/a/very/long/path/that/does/not/fit/DockerDesktop"
	m := loaded(f)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := next.(Model).View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "running (ephemeral)") || strings.Contains(plain, "stopped (ephemeral)") {
		t.Fatalf("ephemeral markers:\n%s", plain)
	}

	var pathStarts []int
	for _, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 80 {
			t.Fatalf("line is %d wide:\n%s", w, line)
		}
		line = ansi.Strip(line)
		var i int
		switch {
		case strings.Contains(line, "DockerDesktop"):
			i = strings.Index(line, "…")
		case strings.Contains(line, "/p/xcode"):
			i = strings.Index(line, "/p/xcode")
		default:
			continue
		}
		pathStarts = append(pathStarts, lipgloss.Width(line[:i]))
	}
	if len(pathStarts) != 2 || pathStarts[0] != pathStarts[1] {
		t.Fatalf("path columns start at %v", pathStarts)
	}
}

// AS-54: ephemeral adds no dashboard prompt and leaves x/E confirmations unchanged.
func TestEphemeralConfirmationsUnchangedAS54(t *testing.T) {
	f := &fakeCtl{snap: snapshot(target.Running), ok: true}
	f.snap.Rows[0].Ephemeral = true
	m := loaded(f)

	m, cmd := press(m, "x")
	if cmd != nil || m.confirming == nil || len(f.calls) != 0 || !strings.Contains(m.View(), "Stop docker?") {
		t.Fatalf("x confirmation changed:\n%s", m.View())
	}
	m, _ = press(m, "n")
	if m.confirming != nil || len(f.calls) != 0 {
		t.Fatal("cancelling an ephemeral stop ran the action")
	}
	m, _ = press(m, "E")
	if m.confirming == nil || !strings.Contains(m.View(), "Stop every target and eject /Volumes/ExternalSSD?") {
		t.Fatalf("E confirmation changed:\n%s", m.View())
	}
}
