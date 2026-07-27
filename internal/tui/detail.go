package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Detail page styles. The focused pane's title is emphasized and its box border
// is highlighted so it's clear where ↑/↓ act; the selected item row is reverse-
// highlighted. Colors reuse the board's palette (39 = section blue, 213 = the
// board's selection pink) so the two views feel like one app.
var (
	detailHeaderStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	detailMetaStyle   = lipgloss.NewStyle().Faint(true)

	paneBox       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	paneBoxFocus  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("213")).Padding(0, 1)
	paneTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	paneTitleFocs = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))

	itemSelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("213"))
	prioHotStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
)

// updateDetail handles key input while the detail page is showing. It is
// strictly read-only: tab moves focus between panes, ↑/↓ (or j/k) either move the
// item selection (Items pane) or scroll the focused pane, and esc/q/left/backspace
// return to the board.
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace", "left":
		m.mode = modeBoard
		return m, nil
	case "tab":
		m.detailFocus = (m.detailFocus + 1) % detailPaneCount
		m.syncDetail()
		return m, nil
	case "shift+tab":
		m.detailFocus = (m.detailFocus + detailPaneCount - 1) % detailPaneCount
		m.syncDetail()
		return m, nil
	case "down", "j":
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = clamp(m.detailItem+1, 0, len(m.detail.items)-1)
			m.syncDetail()
		} else {
			m.detailVP.ScrollDown(1)
		}
		return m, nil
	case "up", "k":
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = clamp(m.detailItem-1, 0, len(m.detail.items)-1)
			m.syncDetail()
		} else {
			m.detailVP.ScrollUp(1)
		}
		return m, nil
	case "g", "home":
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = 0
			m.syncDetail()
		} else {
			m.detailVP.GotoTop()
		}
		return m, nil
	case "G", "end":
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = len(m.detail.items) - 1
			m.syncDetail()
		} else {
			m.detailVP.GotoBottom()
		}
		return m, nil
	}
	return m, nil
}

// resizeDetail sizes the detail viewport to the current window, leaving room for
// the header and footer lines.
func (m *Model) resizeDetail() {
	w := m.width
	if w < 20 {
		w = 80
	}
	h := m.height - 4 // header (1) + blank (1) + footer (1) + margin (1)
	if h < 3 {
		h = 10
	}
	m.detailVP.Width = w
	m.detailVP.Height = h
}

// syncDetail rebuilds the scrollable body from the loaded detail and pushes it
// into the viewport, then nudges the viewport so the selected item stays visible.
func (m *Model) syncDetail() {
	m.detailVP.SetContent(m.detailBody())
	m.ensureItemVisible()
}

// detailBody renders the three panes (Open items, Recent activity, Trajectory)
// as one string for the viewport. The panes are boxed; the focused one gets a
// highlighted border and title.
func (m Model) detailBody() string {
	d := m.detail
	innerW := m.detailVP.Width - 4 // account for box border+padding
	if innerW < 20 {
		innerW = 20
	}

	// Open items
	var items strings.Builder
	if len(d.items) == 0 {
		items.WriteString(emptyStyle.Render("(none)"))
	}
	for i, r := range d.items {
		prio := fmt.Sprintf("p%-2v", str(r["priority"]))
		if str(r["priority"]) == "1" {
			prio = prioHotStyle.Render(prio)
		}
		line := fmt.Sprintf("%-12v %s %-8v %v  (%s old)",
			str(r["id"]), prio, str(r["type"]), str(r["title"]), ageDays(r["created_at"]))
		if m.detailFocus == paneItems && i == m.detailItem {
			line = itemSelStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		items.WriteString(line)
		if i < len(d.items)-1 {
			items.WriteString("\n")
		}
	}

	// Recent activity
	var acts strings.Builder
	if len(d.acts) == 0 {
		acts.WriteString(emptyStyle.Render("(none)"))
	}
	for i, r := range d.acts {
		acts.WriteString(fmt.Sprintf("  [%v] %v  (%s ago)", str(r["kind"]), str(r["summary"]), ageDays(r["occurred_at"])))
		if i < len(d.acts)-1 {
			acts.WriteString("\n")
		}
	}

	// Trajectory
	var traj strings.Builder
	if len(d.stages) == 0 {
		traj.WriteString(emptyStyle.Render("(no recorded transitions)"))
	}
	for i, r := range d.stages {
		reason := ""
		if s := str(r["reason"]); s != "" && s != "<nil>" {
			reason = "  (" + s + ")"
		}
		traj.WriteString(fmt.Sprintf("  %v → %v%s", str(r["from_stage"]), str(r["to_stage"]), reason))
		if i < len(d.stages)-1 {
			traj.WriteString("\n")
		}
	}

	panes := []string{
		m.renderPane(paneItems, fmt.Sprintf("Open items (%d)", len(d.items)), items.String(), innerW),
		m.renderPane(paneActivity, "Recent activity", acts.String(), innerW),
		m.renderPane(paneTrajectory, "Trajectory", traj.String(), innerW),
	}
	return strings.Join(panes, "\n")
}

// renderPane draws one boxed section, highlighting its border and title when it
// is the focused pane.
func (m Model) renderPane(p detailPane, title, body string, innerW int) string {
	box, tstyle := paneBox, paneTitle
	if m.detailFocus == p {
		box, tstyle = paneBoxFocus, paneTitleFocs
	}
	content := tstyle.Render(title) + "\n" + body
	return box.Width(innerW).Render(content)
}

// ensureItemVisible scrolls the viewport so the selected Open item stays in view
// as the selection moves. Best-effort: the Items pane is the first pane, offset
// by the box's top border and title lines.
func (m *Model) ensureItemVisible() {
	if m.detailFocus != paneItems || len(m.detail.items) == 0 {
		return
	}
	// box top border (1) + title (1) = 2 lines before the first item row.
	row := 2 + m.detailItem
	top := m.detailVP.YOffset
	bottom := top + m.detailVP.Height - 1
	if row < top {
		m.detailVP.SetYOffset(row)
	} else if row > bottom {
		m.detailVP.SetYOffset(row - m.detailVP.Height + 1)
	}
}

func (m Model) detailView() string {
	d := m.detail
	if m.detailLoading {
		return fmt.Sprintf("\n  loading %s …\n\n  esc to go back\n", d.c.name)
	}
	if d.err != nil {
		return fmt.Sprintf("\n  error: %v\n\n  esc to go back\n", d.err)
	}

	dot := lipgloss.NewStyle().Foreground(healthColor(d.c.health)).Render("●")
	header := detailHeaderStyle.Render(fmt.Sprintf("%s %s (%s)", dot, d.c.name, d.c.id)) +
		detailMetaStyle.Render(fmt.Sprintf("   stage %s · health %s", pretty(d.c.stage), d.c.health))

	foot := footerStyle.Render("tab focus pane · ↑/↓ select/scroll · g/G top/bottom · esc back · ctrl+c quit")

	// Guard against an unsized viewport (no WindowSizeMsg yet): render the body
	// directly so the page is never blank.
	body := m.detailVP.View()
	if m.detailVP.Height == 0 {
		body = m.detailBody()
	}
	return header + "\n" + body + "\n" + foot + "\n"
}
