// Package tui renders a live, read-only "parade" of accounts across their
// journey stages. Lanes are the journey (onboarding -> renewed); cards are
// customers, colored by health. Arrow keys (or hjkl) move the selection; enter
// drills into a customer detail view. It reads the same data `cs prime`/`cs show`
// do and re-queries on a timer, so it reflects edits made from other terminals.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KazChe/cs/internal/store"
)

// laneOrder is the customer journey, left to right. A stage not listed here
// falls into a trailing "other" lane so nothing is silently dropped.
var laneOrder = []string{"onboarding", "adoption", "expansion", "renewal_risk", "renewed"}

// laneColWidth is the rendered width of one lane column, used to decide how many
// lanes fit on screen. Card content 20 + card padding 2 + card border 2 + lane
// padding 2 = 26.
const laneColWidth = 26

type customer struct {
	id, name, stage, health string
	open                    int
}

type lane struct {
	name  string
	custs []customer
}

type mode int

const (
	modeBoard mode = iota
	modeDetail
)

type detail struct {
	c      customer
	items  []map[string]any
	acts   []map[string]any
	stages []map[string]any
	err    error
}

type loadedMsg struct {
	custs []customer
	err   error
}

type detailMsg struct{ d detail }

type tickMsg time.Time

// Model is the Bubble Tea model backing `cs board`.
type Model struct {
	st       *store.Store
	lanes    []lane
	err      error
	width    int
	height   int
	lastLoad time.Time

	laneIdx int
	cardIdx int
	hoffset int

	mode          mode
	detail        detail
	detailLoading bool
}

func New(st *store.Store) Model { return Model{st: st} }

func (m Model) Init() tea.Cmd { return tea.Batch(load(m.st), tick()) }

func load(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		rows, err := st.Query(
			"SELECT c.id, c.name, c.stage, c.health, " +
				"(SELECT COUNT(*) FROM items i WHERE i.customer_id=c.id AND i.status<>'resolved') AS open_items " +
				"FROM customers c ORDER BY c.name")
		if err != nil {
			return loadedMsg{err: err}
		}
		var out []customer
		for _, r := range rows {
			out = append(out, customer{
				id:     str(r["id"]),
				name:   str(r["name"]),
				stage:  str(r["stage"]),
				health: str(r["health"]),
				open:   toInt(r["open_items"]),
			})
		}
		return loadedMsg{custs: out}
	}
}

func loadDetail(st *store.Store, c customer) tea.Cmd {
	return func() tea.Msg {
		d := detail{c: c}
		var err error
		if d.items, err = st.Query("SELECT id,type,priority,title,created_at FROM items WHERE customer_id=" +
			q(c.id) + " AND status<>'resolved' ORDER BY created_at"); err != nil {
			return detailMsg{detail{c: c, err: err}}
		}
		if d.acts, err = st.Query("SELECT kind,summary,occurred_at FROM activities WHERE customer_id=" +
			q(c.id) + " ORDER BY occurred_at DESC LIMIT 5"); err != nil {
			return detailMsg{detail{c: c, err: err}}
		}
		if d.stages, err = st.Query("SELECT from_stage,to_stage,reason,occurred_at FROM stage_events WHERE customer_id=" +
			q(c.id) + " ORDER BY occurred_at"); err != nil {
			return detailMsg{detail{c: c, err: err}}
		}
		return detailMsg{d}
	}
}

func tick() tea.Cmd {
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.mode == modeDetail {
			switch msg.String() {
			case "esc", "q", "enter", "backspace", "left":
				m.mode = modeBoard
			}
			return m, nil
		}
		switch msg.String() {
		case "q", "esc":
			return m, tea.Quit
		case "r":
			return m, load(m.st)
		case "left", "h":
			m.moveLane(-1)
		case "right", "l":
			m.moveLane(1)
		case "up", "k":
			m.moveCard(-1)
		case "down", "j":
			m.moveCard(1)
		case "enter":
			if l := m.currentLane(); l != nil && len(l.custs) > 0 {
				c := l.custs[m.cardIdx]
				m.mode = modeDetail
				m.detail = detail{c: c}
				m.detailLoading = true
				return m, loadDetail(m.st, c)
			}
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
	case loadedMsg:
		m.err = msg.err
		m.lastLoad = time.Now()
		if msg.err == nil {
			m.lanes = buildLanes(msg.custs)
			m.clampSelection()
		}
	case detailMsg:
		m.detail = msg.d
		m.detailLoading = false
	case tickMsg:
		return m, tea.Batch(load(m.st), tick())
	}
	return m, nil
}

func buildLanes(custs []customer) []lane {
	buckets := map[string][]customer{}
	for _, c := range custs {
		key := c.stage
		if !known(key) {
			key = "other"
		}
		buckets[key] = append(buckets[key], c)
	}
	lanes := make([]lane, 0, len(laneOrder)+1)
	for _, name := range laneOrder {
		lanes = append(lanes, lane{name: name, custs: buckets[name]})
	}
	if len(buckets["other"]) > 0 {
		lanes = append(lanes, lane{name: "other", custs: buckets["other"]})
	}
	return lanes
}

func (m *Model) currentLane() *lane {
	if m.laneIdx < 0 || m.laneIdx >= len(m.lanes) {
		return nil
	}
	return &m.lanes[m.laneIdx]
}

func (m *Model) moveLane(d int) {
	if len(m.lanes) == 0 {
		return
	}
	m.laneIdx = clamp(m.laneIdx+d, 0, len(m.lanes)-1)
	m.cardIdx = clamp(m.cardIdx, 0, max(0, len(m.lanes[m.laneIdx].custs)-1))
	m.ensureVisible()
}

func (m *Model) moveCard(d int) {
	if len(m.lanes) == 0 {
		return
	}
	n := len(m.lanes[m.laneIdx].custs)
	if n == 0 {
		m.cardIdx = 0
		return
	}
	m.cardIdx = clamp(m.cardIdx+d, 0, n-1)
}

func (m *Model) clampSelection() {
	if len(m.lanes) == 0 {
		m.laneIdx, m.cardIdx = 0, 0
		return
	}
	m.laneIdx = clamp(m.laneIdx, 0, len(m.lanes)-1)
	n := len(m.lanes[m.laneIdx].custs)
	if n == 0 {
		m.cardIdx = 0
	} else {
		m.cardIdx = clamp(m.cardIdx, 0, n-1)
	}
	m.ensureVisible()
}

func (m *Model) visibleCount() int {
	if m.width <= 0 {
		return len(m.lanes)
	}
	n := m.width / laneColWidth
	if n < 1 {
		n = 1
	}
	return n
}

func (m *Model) ensureVisible() {
	vc := m.visibleCount()
	if m.laneIdx < m.hoffset {
		m.hoffset = m.laneIdx
	}
	if m.laneIdx >= m.hoffset+vc {
		m.hoffset = m.laneIdx - vc + 1
	}
	maxOff := len(m.lanes) - vc
	if maxOff < 0 {
		maxOff = 0
	}
	m.hoffset = clamp(m.hoffset, 0, maxOff)
}

var (
	laneStyle     = lipgloss.NewStyle().Padding(0, 1)
	laneHeader    = lipgloss.NewStyle().Bold(true).Underline(true)
	laneHeaderSel = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("213"))
	cardStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(20)
	titleStyle    = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	footerStyle   = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	sectionStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	emptyStyle    = lipgloss.NewStyle().Faint(true)
	scrollStyle   = lipgloss.NewStyle().Faint(true).Padding(0, 1)
)

func healthColor(h string) lipgloss.Color {
	switch h {
	case "green":
		return lipgloss.Color("42")
	case "yellow":
		return lipgloss.Color("220")
	case "red":
		return lipgloss.Color("196")
	}
	return lipgloss.Color("245")
}

func (m Model) View() string {
	if m.mode == modeDetail {
		return m.detailView()
	}
	if m.err != nil {
		return fmt.Sprintf("\n  error: %v\n\n  press q to quit\n", m.err)
	}

	vc := m.visibleCount()
	start := m.hoffset
	end := start + vc
	if end > len(m.lanes) {
		end = len(m.lanes)
	}

	cols := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		cols = append(cols, renderLane(m.lanes[i], i == m.laneIdx, m.cardIdx))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	title := titleStyle.Render(fmt.Sprintf("cs board  ·  %d accounts", countCusts(m.lanes)))
	scroll := scrollBar(start, end, len(m.lanes))
	foot := footerStyle.Render(fmt.Sprintf(
		"←/→ lanes · ↑/↓ cards · enter details · r refresh · q quit%s", loadedAt(m.lastLoad)))

	out := title + "\n"
	if scroll != "" {
		out += scroll + "\n"
	}
	return out + board + "\n" + foot + "\n"
}

func (m Model) detailView() string {
	d := m.detail
	if m.detailLoading {
		return fmt.Sprintf("\n  loading %s ...\n\n  esc to go back\n", d.c.name)
	}
	if d.err != nil {
		return fmt.Sprintf("\n  error: %v\n\n  esc to go back\n", d.err)
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("%s (%s)   stage=%s   health=%s",
		d.c.name, d.c.id, d.c.stage, d.c.health)))
	b.WriteString("\n\n")

	b.WriteString(sectionStyle.Render("Open items (oldest first)"))
	b.WriteString("\n")
	if len(d.items) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range d.items {
		b.WriteString(fmt.Sprintf("  %-12v p%-2v %-8v %v  (%s old)\n",
			str(r["id"]), str(r["priority"]), str(r["type"]), str(r["title"]), ageDays(r["created_at"])))
	}

	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Recent activity"))
	b.WriteString("\n")
	if len(d.acts) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range d.acts {
		b.WriteString(fmt.Sprintf("  [%v] %v  (%s ago)\n", str(r["kind"]), str(r["summary"]), ageDays(r["occurred_at"])))
	}

	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("Trajectory"))
	b.WriteString("\n")
	if len(d.stages) == 0 {
		b.WriteString("  (no recorded transitions)\n")
	}
	for _, r := range d.stages {
		reason := ""
		if s := str(r["reason"]); s != "" && s != "<nil>" {
			reason = "  (" + s + ")"
		}
		b.WriteString(fmt.Sprintf("  %v -> %v%s\n", str(r["from_stage"]), str(r["to_stage"]), reason))
	}

	b.WriteString("\n")
	b.WriteString(footerStyle.Render("esc back · ctrl+c quit"))
	b.WriteString("\n")
	return b.String()
}

func renderLane(l lane, selLane bool, selCard int) string {
	hdrStyle := laneHeader
	if selLane {
		hdrStyle = laneHeaderSel
	}
	var b strings.Builder
	b.WriteString(hdrStyle.Render(fmt.Sprintf("%s (%d)", pretty(l.name), len(l.custs))))
	b.WriteString("\n\n")
	if len(l.custs) == 0 {
		b.WriteString(emptyStyle.Render("(none)"))
	}
	for i, c := range l.custs {
		b.WriteString(renderCard(c, selLane && i == selCard))
		b.WriteString("\n")
	}
	return laneStyle.Render(b.String())
}

func renderCard(c customer, selected bool) string {
	st := cardStyle
	bc := healthColor(c.health)
	if selected {
		st = st.Border(lipgloss.DoubleBorder())
		bc = lipgloss.Color("213")
	}
	dot := lipgloss.NewStyle().Foreground(healthColor(c.health)).Render("●")
	head := fmt.Sprintf("%s %s", dot, c.name)
	sub := emptyStyle.Render(fmt.Sprintf("%s · %d open", c.id, c.open))
	return st.BorderForeground(bc).Render(head + "\n" + sub)
}

func scrollBar(start, end, total int) string {
	if start == 0 && end >= total {
		return ""
	}
	left, right := "", ""
	if start > 0 {
		left = fmt.Sprintf("‹ %d more", start)
	}
	if end < total {
		right = fmt.Sprintf("%d more ›", total-end)
	}
	return scrollStyle.Render(fmt.Sprintf("%-12s%s", left, right))
}

func countCusts(lanes []lane) int {
	n := 0
	for _, l := range lanes {
		n += len(l.custs)
	}
	return n
}

func known(stage string) bool {
	for _, s := range laneOrder {
		if s == stage {
			return true
		}
	}
	return false
}

func pretty(s string) string { return strings.ReplaceAll(s, "_", " ") }

func loadedAt(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return "   ·   updated " + t.Format("15:04:05")
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// q quotes and escapes a SQL string literal for the Dolt CLI.
func q(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}

// ageDays renders how old a timestamp value is, e.g. "6d". Empty if unparseable.
func ageDays(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return ""
		}
	}
	return fmt.Sprintf("%dd", int(time.Since(t).Hours()/24))
}
