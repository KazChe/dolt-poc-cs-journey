package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dueSQLLiteral turns a user-entered due date into a SQL literal: NULL for an
// empty string (clear the date), or a quoted YYYY-MM-DD after validation. It
// mirrors the CLI's dueLiteral so the TUI and `cs item due` accept the same
// input.
func dueSQLLiteral(date string) (string, error) {
	if date == "" {
		return "NULL", nil
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", fmt.Errorf("invalid date %q: use YYYY-MM-DD", date)
	}
	return q(date), nil
}

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

	detailStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Padding(0, 1)
	detailPromptStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213")).Padding(0, 1)
)

// selectedItem returns the currently highlighted Open item row, or nil if the
// list is empty or the selection is out of range.
func (m Model) selectedItem() map[string]any {
	if m.detailItem < 0 || m.detailItem >= len(m.detail.items) {
		return nil
	}
	return m.detail.items[m.detailItem]
}

// updateDetail handles key input while the detail page is showing. Navigation
// (tab/↑/↓/g/G) is read-only; the action keys r (resolve) and d (set/clear due)
// mutate the selected item and commit immediately (commit-on-action), then
// reload the detail. While capturing a due date, all keys go to the input.
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.dueEditing {
		return m.updateDueInput(msg)
	}
	switch msg.String() {
	case "esc", "q", "backspace", "left":
		m.mode = modeBoard
		return m, nil
	case "r":
		return m.resolveSelected()
	case "d":
		return m.beginDueEdit()
	case "tab":
		m.detailStatus = ""
		m.detailFocus = (m.detailFocus + 1) % detailPaneCount
		m.syncDetail()
		return m, nil
	case "shift+tab":
		m.detailStatus = ""
		m.detailFocus = (m.detailFocus + detailPaneCount - 1) % detailPaneCount
		m.syncDetail()
		return m, nil
	case "down", "j":
		m.detailStatus = ""
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = clamp(m.detailItem+1, 0, len(m.detail.items)-1)
			m.syncDetail()
		} else {
			m.detailVP.ScrollDown(1)
		}
		return m, nil
	case "up", "k":
		m.detailStatus = ""
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = clamp(m.detailItem-1, 0, len(m.detail.items)-1)
			m.syncDetail()
		} else {
			m.detailVP.ScrollUp(1)
		}
		return m, nil
	case "g", "home":
		m.detailStatus = ""
		if m.detailFocus == paneItems && len(m.detail.items) > 0 {
			m.detailItem = 0
			m.syncDetail()
		} else {
			m.detailVP.GotoTop()
		}
		return m, nil
	case "G", "end":
		m.detailStatus = ""
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

// resolveSelected marks the selected Open item resolved and commits, then
// reloads the detail so the item drops out of the open list.
func (m Model) resolveSelected() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		return m, nil
	}
	id := str(it["id"])
	if err := m.st.Exec(fmt.Sprintf(
		"UPDATE items SET status='resolved', resolved_at=NOW() WHERE id=%s", q(id))); err != nil {
		m.detailStatus = "✗ resolve failed: " + err.Error()
		m.syncDetail()
		return m, nil
	}
	if err := m.st.Commit("item: resolve " + id); err != nil {
		// The UPDATE already landed in the working set; reload so the view
		// reflects the real DB state even though the commit didn't complete.
		m.detailStatus = "✗ commit failed: " + err.Error()
		return m, loadDetail(m.st, m.detail.c)
	}
	m.detailStatus = "✓ resolved " + id
	return m, loadDetail(m.st, m.detail.c)
}

// beginDueEdit opens the inline due-date input for the selected item.
func (m Model) beginDueEdit() (tea.Model, tea.Cmd) {
	if m.selectedItem() == nil {
		return m, nil
	}
	m.dueEditing = true
	m.detailStatus = ""
	m.detailInput.SetValue("")
	m.detailInput.Focus()
	return m, textinputBlink
}

// updateDueInput captures keystrokes while the due-date input is open. Enter
// commits the new date (empty clears it), esc cancels.
func (m Model) updateDueInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.dueEditing = false
		m.detailInput.Blur()
		m.syncDetail()
		return m, nil
	case "enter":
		return m.commitDue()
	}
	var cmd tea.Cmd
	m.detailInput, cmd = m.detailInput.Update(msg)
	return m, cmd
}

// commitDue validates and applies the entered due date to the selected item,
// commits, and reloads. An empty value clears the due date.
func (m Model) commitDue() (tea.Model, tea.Cmd) {
	it := m.selectedItem()
	if it == nil {
		m.dueEditing = false
		m.detailInput.Blur()
		return m, nil
	}
	id := str(it["id"])
	date := strings.TrimSpace(m.detailInput.Value())
	lit, err := dueSQLLiteral(date)
	if err != nil {
		// Keep the input open so the user can correct the date.
		m.detailStatus = "✗ " + err.Error()
		m.syncDetail()
		return m, nil
	}
	m.dueEditing = false
	m.detailInput.Blur()
	if err := m.st.Exec(fmt.Sprintf("UPDATE items SET due_at=%s WHERE id=%s", lit, q(id))); err != nil {
		m.detailStatus = "✗ due update failed: " + err.Error()
		m.syncDetail()
		return m, nil
	}
	msg := "item: due " + id + " cleared"
	if date != "" {
		msg = "item: due " + id + " " + date
	}
	if err := m.st.Commit(msg); err != nil {
		// The UPDATE already landed in the working set; reload so the view
		// reflects the real DB state even though the commit didn't complete.
		m.detailStatus = "✗ commit failed: " + err.Error()
		return m, loadDetail(m.st, m.detail.c)
	}
	if date == "" {
		m.detailStatus = "✓ cleared due on " + id
	} else {
		m.detailStatus = "✓ " + id + " due " + date
	}
	return m, loadDetail(m.st, m.detail.c)
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
		selected := m.detailFocus == paneItems && i == m.detailItem
		// Priority gets a red emphasis only on unselected rows. On the selected
		// row we style the whole line with itemSelStyle instead; nesting an
		// already-styled span inside it would embed an ANSI reset that cuts the
		// selection background short mid-line.
		prio := fmt.Sprintf("p%-2v", str(r["priority"]))
		if !selected && str(r["priority"]) == "1" {
			prio = prioHotStyle.Render(prio)
		}
		line := fmt.Sprintf("%-12v %s %-8v %v  (%s old)",
			str(r["id"]), prio, str(r["type"]), str(r["title"]), ageDays(r["created_at"]))
		if selected {
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

	// Guard against an unsized viewport (no WindowSizeMsg yet): render the body
	// directly so the page is never blank.
	body := m.detailVP.View()
	if m.detailVP.Height == 0 {
		body = m.detailBody()
	}

	// Bottom line: the due-date prompt while editing, else the last action's
	// status, else the key hints.
	var foot string
	switch {
	case m.dueEditing:
		id := ""
		if it := m.selectedItem(); it != nil {
			id = str(it["id"])
		}
		foot = detailPromptStyle.Render("due for "+id+": ") + m.detailInput.View() +
			footerStyle.Render("   enter save · esc cancel")
	case m.detailStatus != "":
		foot = detailStatusStyle.Render(m.detailStatus) +
			footerStyle.Render("   r resolve · d due · esc back")
	default:
		foot = footerStyle.Render("tab pane · ↑/↓ select · r resolve · d due · esc back · ctrl+c quit")
	}
	return header + "\n" + body + "\n" + foot + "\n"
}
