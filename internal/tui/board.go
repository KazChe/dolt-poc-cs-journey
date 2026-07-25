// Package tui renders a live, read-only "parade" of accounts across their
// journey stages. Lanes are the journey (onboarding -> renewed); cards are
// customers, colored by health. It reads the same snapshot `cs prime` does and
// re-queries on a timer, so it reflects edits made from other terminals.
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

type customer struct {
	id, name, stage, health string
	open                    int
}

type loadedMsg struct {
	custs []customer
	err   error
}

type tickMsg time.Time

// Model is the Bubble Tea model backing `cs board`.
type Model struct {
	st       *store.Store
	custs    []customer
	err      error
	width    int
	height   int
	lastLoad time.Time
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

func tick() tea.Cmd {
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			return m, load(m.st)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case loadedMsg:
		m.custs, m.err, m.lastLoad = msg.custs, msg.err, time.Now()
	case tickMsg:
		return m, tea.Batch(load(m.st), tick())
	}
	return m, nil
}

var (
	laneStyle   = lipgloss.NewStyle().Padding(0, 1)
	laneHeader  = lipgloss.NewStyle().Bold(true).Underline(true)
	cardStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(20)
	titleStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	footerStyle = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	emptyStyle  = lipgloss.NewStyle().Faint(true)
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
	if m.err != nil {
		return fmt.Sprintf("\n  error: %v\n\n  press q to quit\n", m.err)
	}

	buckets := map[string][]customer{}
	for _, c := range m.custs {
		key := c.stage
		if !known(key) {
			key = "other"
		}
		buckets[key] = append(buckets[key], c)
	}

	lanes := append([]string{}, laneOrder...)
	if len(buckets["other"]) > 0 {
		lanes = append(lanes, "other")
	}

	cols := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		cols = append(cols, renderLane(lane, buckets[lane]))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	title := titleStyle.Render(fmt.Sprintf("cs board  ·  %d accounts", len(m.custs)))
	foot := footerStyle.Render(fmt.Sprintf(
		"● green  ● yellow  ● red   ·   r refresh   ·   q quit%s", loadedAt(m.lastLoad)))
	return title + "\n" + board + "\n" + foot + "\n"
}

func renderLane(name string, cs []customer) string {
	var b strings.Builder
	b.WriteString(laneHeader.Render(fmt.Sprintf("%s (%d)", pretty(name), len(cs))))
	b.WriteString("\n\n")
	if len(cs) == 0 {
		b.WriteString(emptyStyle.Render("(none)"))
	}
	for _, c := range cs {
		b.WriteString(renderCard(c))
		b.WriteString("\n")
	}
	return laneStyle.Render(b.String())
}

func renderCard(c customer) string {
	dot := lipgloss.NewStyle().Foreground(healthColor(c.health)).Render("●")
	head := fmt.Sprintf("%s %s", dot, c.name)
	sub := emptyStyle.Render(fmt.Sprintf("%s · %d open", c.id, c.open))
	return cardStyle.BorderForeground(healthColor(c.health)).Render(head + "\n" + sub)
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
