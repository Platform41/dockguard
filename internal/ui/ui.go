// Package ui is the terminal dashboard, `dockguard ui` (docs/specs/multi-target.md §2.4, D-018).
// It only draws and handles keys; every probe and action goes through a Controller, which the app
// package implements with the same code as the CLI commands.
package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/platform41/dockguard/internal/target"
)

// RefreshInterval is how often the dashboard probes every target. A variable so tests can shorten it.
var RefreshInterval = 3 * time.Second

// Row is one target as the dashboard shows it.
type Row struct {
	Name, Kind, Path string
	State            target.State
	// Err explains an unknown state.
	Err string
}

// Snapshot is one probe of the drive and every target.
type Snapshot struct {
	ConfigPath string
	Drive      string
	Mounted    bool
	// UsedPct, FreeGB and TotalGB are zero when the drive isn't mounted.
	UsedPct         float64
	FreeGB, TotalGB uint64
	Rows            []Row
}

// Action is something the dashboard can ask the controller to do.
type Action int

const (
	Start Action = iota
	Stop
	Check
	StopAllEject
)

func (a Action) String() string {
	switch a {
	case Start:
		return "start"
	case Stop:
		return "stop"
	case Check:
		return "check"
	default:
		return "stop --all --eject"
	}
}

// Controller probes and acts. Implementations must be safe to call from a background goroutine.
type Controller interface {
	Snapshot() Snapshot
	// Run performs the action (target is empty for StopAllEject) and returns whether it succeeded
	// and the text the matching CLI command would have printed.
	Run(action Action, target string) (ok bool, output string)
}

// Blockers lists the targets that stop the drive from being ejected (spec §2.3): every target
// that isn't confirmed stopped.
func Blockers(s Snapshot) []string {
	var blockers []string
	for _, r := range s.Rows {
		if r.State != target.Stopped {
			blockers = append(blockers, fmt.Sprintf("%s (%s)", r.Name, r.State))
		}
	}
	return blockers
}

// Messages.
type (
	snapshotMsg Snapshot
	tickMsg     struct{}
	actionMsg   struct {
		action Action
		target string
		ok     bool
		output string
	}
)

// Model is the dashboard's state.
type Model struct {
	ctl        Controller
	snap       Snapshot
	loaded     bool
	selected   int
	refreshing bool
	busy       bool
	confirming *pending
	// queued is an action waiting for an in-flight probe to finish.
	queued     *pending
	lastTitle  string
	lastOK     bool
	lastOutput string
	width      int
}

type pending struct {
	action Action
	target string
}

// New returns a dashboard model.
func New(ctl Controller) Model { return Model{ctl: ctl, refreshing: true} }

func (m Model) Init() tea.Cmd { return tea.Batch(m.refresh(), tick()) }

func (m Model) refresh() tea.Cmd {
	ctl := m.ctl
	return func() tea.Msg { return snapshotMsg(ctl.Snapshot()) }
}

func tick() tea.Cmd {
	return tea.Tick(RefreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) run(a Action, name string) tea.Cmd {
	ctl := m.ctl
	return func() tea.Msg {
		ok, out := ctl.Run(a, name)
		return actionMsg{action: a, target: name, ok: ok, output: out}
	}
}

// dispatch starts an action, or queues it until the probe in flight has finished: probes and
// actions never overlap, so an older snapshot can't overwrite the result of a newer action.
func (m Model) dispatch(a Action, name string) (Model, tea.Cmd) {
	m.busy = true
	if m.refreshing {
		m.queued = &pending{action: a, target: name}
		return m, nil
	}
	return m, m.run(a, name)
}

func (m Model) selectedName() string {
	if m.selected >= 0 && m.selected < len(m.snap.Rows) {
		return m.snap.Rows[m.selected].Name
	}
	return ""
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case snapshotMsg:
		m.snap, m.loaded, m.refreshing = Snapshot(msg), true, false
		if m.selected >= len(m.snap.Rows) {
			m.selected = len(m.snap.Rows) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		// An action that was waiting for this probe runs now.
		if q := m.queued; q != nil {
			m.queued = nil
			return m, m.run(q.action, q.target)
		}
		return m, nil

	case tickMsg:
		// Never start a probe while the previous one, or an action, is still running.
		if m.refreshing || m.busy {
			return m, tick()
		}
		m.refreshing = true
		return m, tea.Batch(m.refresh(), tick())

	case actionMsg:
		m.busy = false
		m.lastOK, m.lastOutput = msg.ok, strings.TrimRight(msg.output, "\n")
		m.lastTitle = msg.action.String()
		if msg.target != "" {
			m.lastTitle += " " + msg.target
		}
		m.refreshing = true
		return m, m.refresh()

	case tea.KeyMsg:
		// A burst of typed or pasted characters arrives as one message ("jj"); handle each
		// character as its own key press. A confirmation must be its own key press, so the
		// burst stops at a prompt (or at an answer to one): pasted "Eye" can't confirm an eject.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			model := m
			for _, r := range msg.Runes {
				wasConfirming := model.confirming != nil
				next, cmd := model.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				model = next.(Model)
				cmds = append(cmds, cmd)
				if wasConfirming || model.confirming != nil {
					break
				}
			}
			return model, tea.Batch(cmds...)
		}
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

	// A pending confirmation takes the next key: "y" runs it, anything else cancels (AS-33).
	if m.confirming != nil {
		p := m.confirming
		m.confirming = nil
		if k == "y" || k == "Y" {
			return m.dispatch(p.action, p.target)
		}
		m.lastTitle, m.lastOK, m.lastOutput = "cancelled", true, ""
		return m, nil
	}

	switch k {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.snap.Rows)-1 {
			m.selected++
		}
	case "r":
		if !m.refreshing && !m.busy {
			m.refreshing = true
			return m, m.refresh()
		}
	}

	if m.busy || !m.loaded {
		return m, nil
	}
	name := m.selectedName()
	switch k {
	case "s":
		if name != "" {
			return m.dispatch(Start, name)
		}
	case "c":
		if name != "" {
			return m.dispatch(Check, name)
		}
	case "x":
		if name != "" {
			m.confirming = &pending{action: Stop, target: name}
		}
	case "E":
		m.confirming = &pending{action: StopAllEject}
	}
	return m, nil
}

// Styles.
var (
	dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	title   = lipgloss.NewStyle().Bold(true)
	good    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	bad     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	boxed   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	confirm = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
)

func stateMark(s target.State) string {
	switch s {
	case target.Running:
		return good.Render("●")
	case target.Partial:
		return warn.Render("◐")
	case target.Stopped:
		return dim.Render("○")
	default:
		return bad.Render("?")
	}
}

func stateText(s target.State) string {
	switch s {
	case target.Running:
		return good.Render("running")
	case target.Partial:
		return warn.Render("partial")
	case target.Stopped:
		return dim.Render("stopped")
	default:
		return bad.Render("unknown")
	}
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	var b strings.Builder

	head := " DockGuard "
	if name := filepath.Base(m.snap.Drive); m.loaded && name != "." && name != "/" && name != "" {
		head = fmt.Sprintf(" DockGuard · %s ", name)
	}
	pad := (width - lipgloss.Width(head)) / 2
	if pad < 0 {
		pad = 0
	}
	b.WriteString(dim.Render(strings.Repeat("─", pad)) + title.Render(head) + dim.Render(strings.Repeat("─", max(0, width-pad-lipgloss.Width(head)))) + "\n")

	if !m.loaded {
		b.WriteString("\n " + dim.Render("Checking the drive and every target…") + "\n")
		return b.String()
	}

	// Drive.
	if m.snap.Mounted {
		bar := usageBar(m.snap.UsedPct, 14)
		usage := fmt.Sprintf("%.0f%% used · %d GB free", m.snap.UsedPct, m.snap.FreeGB)
		b.WriteString(fit(width,
			fmt.Sprintf(" %s  %s   %s mounted   %s  %s", title.Render("Drive"), m.snap.Drive, good.Render("●"), bar, usage),
			fmt.Sprintf(" %s  %s   %s mounted   %s", title.Render("Drive"), m.snap.Drive, good.Render("●"), usage),
			fmt.Sprintf(" %s  %s  %s %s", title.Render("Drive"), m.snap.Drive, good.Render("●"), usage),
		) + "\n")
	} else {
		b.WriteString(fmt.Sprintf(" %s  %s   %s\n", title.Render("Drive"), m.snap.Drive, bad.Render("○ not mounted")))
	}

	// Eject readiness (AS-32).
	switch blockers := Blockers(m.snap); {
	case !m.snap.Mounted:
		b.WriteString(fmt.Sprintf(" %s  %s\n", title.Render("Eject"), dim.Render("nothing to eject")))
	case len(blockers) == 0:
		b.WriteString(fmt.Sprintf(" %s  %s\n", title.Render("Eject"), good.Render("✓ safe to eject")))
	default:
		b.WriteString(wrapList(" "+title.Render("Eject")+"  "+bad.Render("✗ blocked by:")+" ", blockers, width))
	}
	b.WriteString("\n")

	// Targets.
	nameW, kindW := 4, 4
	for _, r := range m.snap.Rows {
		nameW, kindW = max(nameW, len(r.Name)), max(kindW, len(r.Kind))
	}
	for i, r := range m.snap.Rows {
		marker := " "
		if i == m.selected {
			marker = accent.Render("▌")
		}
		// Pad before styling: colour codes would otherwise count towards the column width.
		prefix := fmt.Sprintf(" %s%%s %-*s  %s  ", marker, nameW, r.Name, dim.Render(fmt.Sprintf("%-*s", kindW, r.Kind)))
		mark, state := stateMark(r.State), padState(stateText(r.State))
		if !m.snap.Mounted {
			mark, state = dim.Render("·"), dim.Render("drive not mounted")
		}
		line := fmt.Sprintf(prefix, mark) + state + "  "
		b.WriteString(line + dim.Render(truncateLeft(r.Path, width-lipgloss.Width(line)-1)) + "\n")
	}
	if len(m.snap.Rows) > 0 {
		if r := m.snap.Rows[m.selected]; m.snap.Mounted && r.State == target.Unknown && r.Err != "" {
			b.WriteString("\n " + bad.Render(r.Name+": "+r.Err) + "\n")
		}
	}

	// Last action.
	if m.queued != nil {
		b.WriteString("\n " + warn.Render("Waiting for the current check to finish, then "+m.queued.action.String()+"…") + "\n")
	} else if m.busy {
		b.WriteString("\n " + warn.Render("Working…") + "\n")
	} else if m.lastTitle != "" {
		status := good.Render("✓")
		if !m.lastOK {
			status = bad.Render("✗")
		}
		body := status + " " + title.Render(m.lastTitle)
		if m.lastOutput != "" {
			body += "\n" + lastLines(m.lastOutput, 12)
		}
		b.WriteString("\n" + boxed.Width(max(20, width-4)).Render(body) + "\n")
	}

	// Footer.
	b.WriteString("\n")
	if m.confirming != nil {
		what := "Stop " + m.confirming.target
		if m.confirming.action == StopAllEject {
			what = "Stop every target and eject " + m.snap.Drive
		}
		short := "Stop " + m.confirming.target + "?"
		if m.confirming.action == StopAllEject {
			short = "Stop every target and eject the drive?"
		}
		b.WriteString(fit(width,
			" "+confirm.Render(what+"?")+" "+dim.Render("y = yes · any other key = cancel"),
			" "+confirm.Render(what+"?")+" "+dim.Render("y / any key = cancel"),
			" "+confirm.Render(short)+" "+dim.Render("y / any key = cancel"),
		) + "\n")
	} else {
		b.WriteString(" " + dim.Render(footer(width-2)) + "\n")
	}
	return b.String()
}

// wrapList writes items after a prefix, separated by ", ", wrapping onto lines indented under the
// first item so every item stays visible within the width.
func wrapList(prefix string, items []string, width int) string {
	indent := strings.Repeat(" ", lipgloss.Width(prefix))
	var b strings.Builder
	line := prefix
	for i, item := range items {
		sep := ""
		if i < len(items)-1 {
			sep = ","
		}
		piece := item + sep
		if i > 0 && lipgloss.Width(line)+1+lipgloss.Width(piece) > width {
			b.WriteString(strings.TrimRight(line, " ") + "\n")
			line = indent
		} else if i > 0 {
			line += " "
		}
		line += piece
	}
	b.WriteString(line + "\n")
	return b.String()
}

// fit returns the first line that fits the width, or the last one when none does.
func fit(width int, lines ...string) string {
	for _, l := range lines {
		if lipgloss.Width(l) <= width {
			return l
		}
	}
	return lines[len(lines)-1]
}

// footer fits the key help to the terminal width, dropping the least important keys first.
func footer(width int) string {
	full := "↑/k ↓/j move · s start · x stop · c check · E stop all + eject · r refresh · q quit"
	short := "↑↓ move · s start · x stop · c check · E eject all · r refresh · q quit"
	shortest := "↑↓ · s start · x stop · E eject all · q quit"
	for _, f := range []string{full, short} {
		if lipgloss.Width(f) <= width {
			return f
		}
	}
	return shortest
}

// truncateLeft keeps the end of a path, which is the part that tells targets apart.
func truncateLeft(s string, width int) string {
	if width <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}

func padState(s string) string {
	return s + strings.Repeat(" ", max(0, 7-lipgloss.Width(s)))
}

func usageBar(pct float64, width int) string {
	filled := int(pct/100*float64(width) + 0.5)
	filled = min(max(filled, 0), width)
	return warn.Render(strings.Repeat("▓", filled)) + dim.Render(strings.Repeat("░", width-filled))
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append([]string{dim.Render(fmt.Sprintf("… %d earlier lines", len(lines)-n))}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

// Run opens the dashboard and returns when the user quits.
func Run(ctl Controller) error {
	_, err := tea.NewProgram(New(ctl), tea.WithAltScreen()).Run()
	return err
}
