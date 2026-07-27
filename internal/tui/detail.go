package tui

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// editKind identifies which inline action is currently capturing text in
// detailInput. editNone means no input is open (normal navigation).
type editKind int

const (
	editNone  editKind = iota
	editDue            // set/clear due date on the selected item
	editAdd            // add a new item (title)
	editNote           // append a note (summary)
	editStage          // advance the account to a new stage
)

// knownStages is the ordered journey the stage action's ('s') dropdown lists;
// mirrors laneOrder so the TUI offers the same stage names the board shows.
var knownStages = laneOrder

// itemTypes / itemPriorities are the fixed enums the add form cycles through,
// matching `cs item add`'s accepted values (type default "action", priority 2).
var (
	itemTypes      = []string{"bug", "feature", "question", "action"}
	itemPriorities = []int{1, 2, 3}
)

// addStep walks the 'a' add-item form: pick a type, pick a priority, type the
// title, then an optional due date. The type/priority steps are selectors
// (←/→ or the listed keys); the title/due steps use detailInput.
type addStep int

const (
	stepType addStep = iota
	stepPriority
	stepTitle
	stepDue
)

// addForm holds the in-progress state of the add-item form. typeIdx/prioIdx
// index into itemTypes/itemPriorities; title/due are captured via detailInput
// as their steps become active.
type addForm struct {
	step    addStep
	typeIdx int
	prioIdx int // defaults to index of priority 2 (see beginAddForm)
	title   string
	due     string
}

// idAlphabet + newID mirror the CLI's id generation (cmd/util.go) so ids created
// from the TUI look the same as ids created from `cs`.
const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func newID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%06x", prefix, time.Now().UnixNano()&0xffffff)
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return prefix + "-" + string(b)
}

// dueAnnotation renders an item's due_at as a compact suffix relative to today,
// mirroring the CLI's dueAnnotation: "due <date>" when upcoming, "⚠ overdue
// <date>" once the date has passed (today still counts as due). The bool is
// true when overdue, so the caller can color it. Empty string for a null/unset
// date so the caller omits it.
func dueAnnotation(v any) (text string, overdue bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	var d time.Time
	var err error
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if d, err = time.Parse(layout, s); err == nil {
			break
		}
	}
	if err != nil {
		return "", false
	}
	day := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if day.Before(today) {
		return "⚠ overdue " + day.Format("2006-01-02"), true
	}
	return "due " + day.Format("2006-01-02"), false
}

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
	dueSoonStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // upcoming due date (amber)

	// fieldHighlight backgrounds the header's stage field so it's visually
	// distinct as the value the stage action (s) updates. healthField is the
	// same treatment for health, with the foreground colored by healthColor at
	// render time.
	fieldHighlight = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true).Padding(0, 1)
	healthField    = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true).Padding(0, 1)
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
// (tab/↑/↓/g/G) is read-only; the action keys mutate and commit immediately
// (commit-on-action), then reload the detail: r resolve, d due, a add item,
// n note, s advance stage. While an inline input is open, all keys go to it.
func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editKind != editNone {
		return m.updateActionInput(msg)
	}
	switch msg.String() {
	case "esc", "q", "backspace", "left":
		m.mode = modeBoard
		return m, nil
	// Action keys are scoped to the pane they belong to, matching the footer
	// hints: item actions on Open items, note on Recent activity, stage on
	// Trajectory. A key pressed on the wrong pane is a no-op.
	case "r":
		if m.detailFocus == paneItems {
			return m.resolveSelected()
		}
		return m, nil
	case "d":
		if m.detailFocus == paneItems {
			return m.beginEdit(editDue)
		}
		return m, nil
	case "a":
		if m.detailFocus == paneItems {
			return m.beginEdit(editAdd)
		}
		return m, nil
	case "n":
		if m.detailFocus == paneActivity {
			return m.beginEdit(editNote)
		}
		return m, nil
	case "s":
		if m.detailFocus == paneTrajectory {
			return m.beginEdit(editStage)
		}
		return m, nil
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

// editPrompt returns the label shown before the inline input for each action.
func editPrompt(k editKind, m Model) string {
	switch k {
	case editDue:
		id := ""
		if it := m.selectedItem(); it != nil {
			id = str(it["id"])
		}
		return "due for " + id + " (YYYY-MM-DD, empty clears)"
	// editAdd and editStage have their own dropdown footers (addFormFoot /
	// stageFoot), not a single-line prompt.
	case editNote:
		return "note"
	}
	return ""
}

// beginEdit opens the inline input for the given action. Actions that operate on
// the selected item (due) require a selection; add/note/stage act on the account
// and don't. editAdd starts the multi-step form (beginAddForm) rather than a
// single input.
func (m Model) beginEdit(k editKind) (tea.Model, tea.Cmd) {
	if k == editDue && m.selectedItem() == nil {
		return m, nil
	}
	if k == editAdd {
		return m.beginAddForm()
	}
	if k == editStage {
		// Stage is a fixed enum, so it's a dropdown selector (like the add
		// form's type/priority) rather than free text. Start on the current
		// stage highlighted so the list reads as "where you are → pick next".
		m.editKind = editStage
		m.detailStatus = ""
		m.stageIdx = stageIndex(m.detail.c.stage)
		m.detailInput.Blur() // the selector doesn't use the text input
		return m, nil
	}
	m.editKind = k
	m.detailStatus = ""
	m.detailInput.SetValue("")
	// A due date is exactly 10 chars; titles/notes need real room. Set the cap
	// and placeholder per action so the shared input doesn't truncate free text
	// or show a stale hint.
	if k == editDue {
		m.detailInput.CharLimit = 10
		m.detailInput.Placeholder = "YYYY-MM-DD"
	} else {
		m.detailInput.CharLimit = 255
		m.detailInput.Placeholder = ""
	}
	m.detailInput.Focus()
	return m, textinputBlink
}

// beginAddForm starts the add-item form on its first step (type selector), with
// priority defaulting to 2 to match `cs item add`.
func (m Model) beginAddForm() (tea.Model, tea.Cmd) {
	m.editKind = editAdd
	m.detailStatus = ""
	m.addForm = addForm{step: stepType, typeIdx: 3, prioIdx: 1} // type=action, priority=2
	m.detailInput.SetValue("")
	m.detailInput.Blur() // selector steps don't use the text input
	return m, nil
}

// updateActionInput captures keystrokes while any inline action input is open.
// The add form (editAdd) has its own multi-step handler; the other actions are
// single-input: enter applies, esc cancels.
func (m Model) updateActionInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editKind == editAdd {
		return m.updateAddForm(msg)
	}
	if m.editKind == editStage {
		return m.updateStageSelect(msg)
	}
	switch msg.String() {
	case "esc":
		m.editKind = editNone
		m.detailInput.Blur()
		m.syncDetail()
		return m, nil
	case "enter":
		return m.commitAction()
	}
	var cmd tea.Cmd
	m.detailInput, cmd = m.detailInput.Update(msg)
	return m, cmd
}

// updateStageSelect drives the 's' stage dropdown: ↑/↓ (and vi keys) move the
// highlight through knownStages, enter commits the chosen stage, esc cancels.
func (m Model) updateStageSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editKind = editNone
		m.syncDetail()
		return m, nil
	case "up", "k", "left", "h":
		m.stageIdx = (m.stageIdx + len(knownStages) - 1) % len(knownStages)
		return m, nil
	case "down", "j", "right", "l", "tab":
		m.stageIdx = (m.stageIdx + 1) % len(knownStages)
		return m, nil
	case "enter":
		return m.commitAction()
	}
	return m, nil
}

// updateAddForm drives the add-item form's steps. esc cancels the whole form
// from any step. Type/priority steps are selectors (←/→ or the number/letter
// keys); title/due steps use detailInput. Enter advances, and on the final step
// (due) commits.
func (m Model) updateAddForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.editKind = editNone
		m.detailInput.Blur()
		m.syncDetail()
		return m, nil
	}
	switch m.addForm.step {
	case stepType:
		switch msg.String() {
		case "up", "k", "left", "h":
			m.addForm.typeIdx = (m.addForm.typeIdx + len(itemTypes) - 1) % len(itemTypes)
		case "down", "j", "right", "l", "tab":
			m.addForm.typeIdx = (m.addForm.typeIdx + 1) % len(itemTypes)
		case "enter":
			m.addForm.step = stepPriority
		}
		return m, nil
	case stepPriority:
		switch msg.String() {
		case "up", "k", "left", "h":
			m.addForm.prioIdx = (m.addForm.prioIdx + len(itemPriorities) - 1) % len(itemPriorities)
		case "down", "j", "right", "l", "tab":
			m.addForm.prioIdx = (m.addForm.prioIdx + 1) % len(itemPriorities)
		case "1", "2", "3":
			m.addForm.prioIdx = int(msg.String()[0] - '1')
		case "enter":
			m.addForm.step = stepTitle
			m.detailInput.SetValue("")
			m.detailInput.CharLimit = 255
			m.detailInput.Placeholder = ""
			m.detailInput.Focus()
			return m, textinputBlink
		}
		return m, nil
	case stepTitle:
		if msg.String() == "enter" {
			title := strings.TrimSpace(m.detailInput.Value())
			if title == "" {
				m.detailStatus = "✗ a title is required"
				m.syncDetail()
				return m, nil
			}
			m.addForm.title = title
			m.detailStatus = ""
			m.addForm.step = stepDue
			m.detailInput.SetValue("")
			m.detailInput.CharLimit = 10
			m.detailInput.Placeholder = "YYYY-MM-DD (optional)"
			return m, nil
		}
		var cmd tea.Cmd
		m.detailInput, cmd = m.detailInput.Update(msg)
		return m, cmd
	case stepDue:
		if msg.String() == "enter" {
			return m.commitAddForm()
		}
		var cmd tea.Cmd
		m.detailInput, cmd = m.detailInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// commitAddForm validates the optional due date, inserts the item with the
// chosen type/priority/title/due, commits, and reloads. A bad due date keeps the
// form on the due step so the user can fix it.
func (m Model) commitAddForm() (tea.Model, tea.Cmd) {
	f := m.addForm
	itemType := itemTypes[f.typeIdx]
	prio := itemPriorities[f.prioIdx]
	due := strings.TrimSpace(m.detailInput.Value())
	dueLit, err := dueSQLLiteral(due)
	if err != nil {
		m.detailStatus = "✗ " + err.Error()
		m.syncDetail()
		return m, nil
	}

	m.editKind = editNone
	m.detailInput.Blur()

	id := newID("itm")
	query := fmt.Sprintf(
		"INSERT INTO items (id,customer_id,type,title,priority,status,due_at) VALUES (%s,%s,%s,%s,%d,'open',%s)",
		q(id), q(m.detail.c.id), q(itemType), q(f.title), prio, dueLit)
	if err := m.st.Exec(query); err != nil {
		m.detailStatus = "✗ add failed: " + err.Error()
		m.syncDetail()
		return m, nil
	}
	if err := m.st.Commit("item: " + id + " " + f.title + " (" + m.detail.c.id + ")"); err != nil {
		m.detailStatus = "✗ commit failed: " + err.Error()
		return m, loadDetail(m.st, m.detail.c)
	}
	m.detailStatus = fmt.Sprintf("✓ added %s [%s p%d]", id, itemType, prio)
	return m, loadDetail(m.st, m.detail.c)
}

// commitAction validates the input for the active editKind, runs the write +
// commit, and reloads the detail. Validation errors keep the input open so the
// user can correct; a nil error closes it. SQL mirrors the equivalent cs CLI
// command exactly.
func (m Model) commitAction() (tea.Model, tea.Cmd) {
	k := m.editKind
	val := strings.TrimSpace(m.detailInput.Value())

	var query, commitMsg, okStatus string
	var multi bool // true when query has multiple statements (needs ExecScript)
	switch k {
	case editDue:
		it := m.selectedItem()
		if it == nil {
			m.editKind = editNone
			m.detailInput.Blur()
			return m, nil
		}
		id := str(it["id"])
		lit, err := dueSQLLiteral(val)
		if err != nil {
			m.detailStatus = "✗ " + err.Error()
			m.syncDetail()
			return m, nil
		}
		query = fmt.Sprintf("UPDATE items SET due_at=%s WHERE id=%s", lit, q(id))
		if val == "" {
			commitMsg, okStatus = "item: due "+id+" cleared", "✓ cleared due on "+id
		} else {
			commitMsg, okStatus = "item: due "+id+" "+val, "✓ "+id+" due "+val
		}
	// editAdd is handled by the multi-step add form (updateAddForm/commitAddForm),
	// not here.
	case editNote:
		if val == "" {
			m.detailStatus = "✗ a note is required"
			m.syncDetail()
			return m, nil
		}
		id := newID("act")
		query = fmt.Sprintf(
			"INSERT INTO activities (id,customer_id,kind,summary,occurred_at) VALUES (%s,%s,'note',%s,NOW())",
			q(id), q(m.detail.c.id), q(val))
		commitMsg = "note: " + m.detail.c.id + " - " + val
		okStatus = "✓ noted"
	case editStage:
		// The stage comes from the dropdown selection (knownStages[stageIdx]),
		// not the text input — so it's always a valid stage by construction.
		to := knownStages[m.stageIdx]
		val = to // keep the header-refresh below (which uses val) in sync
		from := m.detail.c.stage
		if to == from {
			m.detailStatus = "✗ already in " + to
			m.syncDetail()
			return m, nil
		}
		id := newID("stg")
		// Advancing the stage also moves health to the value that stage implies
		// (healthForStage), so the two header fields stay in sync in one commit.
		health := healthForStage(to)
		// Mirror `cs stage`: record the transition and update the customer's
		// stage, and set health to match the new stage.
		query = fmt.Sprintf(
			"INSERT INTO stage_events (id,customer_id,from_stage,to_stage,reason,occurred_at) VALUES (%s,%s,%s,%s,'',NOW());\n"+
				"UPDATE customers SET stage=%s, health=%s, updated_at=NOW() WHERE id=%s;",
			q(id), q(m.detail.c.id), q(from), q(to), q(to), q(health), q(m.detail.c.id))
		multi = true
		commitMsg = "stage: " + m.detail.c.id + " " + from + "->" + to
		okStatus = "✓ " + from + " → " + to
	default:
		m.editKind = editNone
		return m, nil
	}

	m.editKind = editNone
	m.detailInput.Blur()

	exec := m.st.Exec
	if multi {
		exec = m.st.ExecScript // multi-statement (stage)
	}
	if err := exec(query); err != nil {
		m.detailStatus = "✗ failed: " + err.Error()
		m.syncDetail()
		return m, nil
	}
	// Advancing the stage changes the customer's stage and health; reflect
	// both in the header before the reload lands.
	if k == editStage {
		m.detail.c.stage = val
		m.detail.c.health = healthForStage(val)
	}
	if err := m.st.Commit(commitMsg); err != nil {
		// The write already landed in the working set; reload to show real state.
		m.detailStatus = "✗ commit failed: " + err.Error()
		return m, loadDetail(m.st, m.detail.c)
	}
	m.detailStatus = okStatus
	return m, loadDetail(m.st, m.detail.c)
}

// stageIndex returns the position of stage s in knownStages, or 0 (the first
// stage) if s isn't a known stage. Used to seed the 's' dropdown highlight on
// the account's current stage.
func stageIndex(s string) int {
	for i, k := range knownStages {
		if k == s {
			return i
		}
	}
	return 0
}

// healthForStage maps a journey stage to the health the account should carry
// once it reaches that stage. Advancing the stage (s) applies this so health
// tracks stage automatically instead of drifting. Any unmapped stage falls
// back to yellow (neutral) rather than leaving stale health behind.
func healthForStage(stage string) string {
	switch stage {
	case "onboarding":
		return "yellow"
	case "adoption", "expansion", "renewed":
		return "green"
	case "renewal_risk":
		return "red"
	}
	return "yellow"
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
		// Show the due date when set: "due <date>" upcoming, "⚠ overdue <date>"
		// past — same convention as `cs show` / `cs item ls`.
		if due, overdue := dueAnnotation(r["due_at"]); due != "" {
			if selected {
				line += "  " + due // whole row is styled below; don't nest a color
			} else if overdue {
				line += "  " + prioHotStyle.Render(due)
			} else {
				line += "  " + dueSoonStyle.Render(due)
			}
		}
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
	// stage + health are highlighted so it's clear they're the fields a stage
	// change (s) touches — both move together (health tracks stage via
	// healthForStage).
	stageBadge := fieldHighlight.Render("stage " + pretty(d.c.stage))
	healthBadge := healthField.Foreground(healthColor(d.c.health)).Render("health " + d.c.health)
	header := detailHeaderStyle.Render(fmt.Sprintf("%s %s (%s)", dot, d.c.name, d.c.id)) +
		"   " + stageBadge + " " + healthBadge

	// Guard against an unsized viewport (no WindowSizeMsg yet): render the body
	// directly so the page is never blank.
	body := m.detailVP.View()
	if m.detailVP.Height == 0 {
		body = m.detailBody()
	}

	// Bottom region: the add form's current step (may be multi-line for the
	// type/priority dropdowns), else the active action's prompt, else the last
	// action's status + pane-scoped hints, else the pane-scoped key hints.
	// Actions are scoped to the focused pane so item actions don't show while
	// you're tabbed onto Activity/Trajectory.
	var foot string
	switch {
	case m.editKind == editAdd:
		foot = m.addFormFoot()
	case m.editKind == editStage:
		foot = m.stageFoot()
	case m.editKind != editNone:
		foot = detailPromptStyle.Render(editPrompt(m.editKind, m)+": ") + m.detailInput.View() +
			footerStyle.Render("   enter save · esc cancel")
	case m.detailStatus != "":
		foot = detailStatusStyle.Render(m.detailStatus) + footerStyle.Render("   "+m.paneActions())
	default:
		foot = footerStyle.Render("tab pane · ↑/↓ select/scroll · " + m.paneActions() + " · esc back")
	}

	// Keep header + body + foot within the window: a multi-line foot (the add
	// dropdowns) borrows lines from the bottom of the scrollable body so the
	// layout doesn't overflow the alt-screen.
	if extra := strings.Count(foot, "\n"); extra > 0 && m.detailVP.Height > 0 {
		body = trimTrailingLines(body, extra)
	}
	return header + "\n" + body + "\n" + foot + "\n"
}

// trimTrailingLines drops the last n lines of s (used to make room for a
// multi-line footer without overflowing the fixed-height view).
func trimTrailingLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if n >= len(lines) {
		return ""
	}
	return strings.Join(lines[:len(lines)-n], "\n")
}

// paneActions returns the action-key hints valid for the currently focused pane.
// Item actions (resolve/due/add) belong to Open items; note to Recent activity;
// stage to Trajectory. This keeps the footer honest about what a keypress does
// where the focus is.
func (m Model) paneActions() string {
	switch m.detailFocus {
	case paneItems:
		return "r resolve · d due · a add"
	case paneActivity:
		return "n note"
	case paneTrajectory:
		return "s stage"
	}
	return ""
}

// addFormFoot renders the add-item form's current step in the footer: the
// type/priority steps show the enum options with the current choice highlighted;
// the title/due steps show the text input. A validation error (m.detailStatus)
// is prepended so the user sees why the step didn't advance.
func (m Model) addFormFoot() string {
	f := m.addForm
	var b strings.Builder
	if m.detailStatus != "" {
		b.WriteString(detailStatusStyle.Render(m.detailStatus) + "  ")
	}
	switch f.step {
	case stepType:
		b.WriteString(detailPromptStyle.Render("type ▾") + footerStyle.Render("   ↑/↓ choose · enter next · esc cancel") + "\n")
		b.WriteString(renderDropdown(itemTypes, f.typeIdx))
	case stepPriority:
		b.WriteString(detailPromptStyle.Render(fmt.Sprintf("[%s]  priority ▾", itemTypes[f.typeIdx])) +
			footerStyle.Render("   ↑/↓ or 1-3 · enter next · esc cancel") + "\n")
		labels := make([]string, len(itemPriorities))
		for i, p := range itemPriorities {
			labels[i] = fmt.Sprintf("p%d", p)
		}
		b.WriteString(renderDropdown(labels, f.prioIdx))
	case stepTitle:
		b.WriteString(detailPromptStyle.Render(fmt.Sprintf("[%s p%d]  title: ", itemTypes[f.typeIdx], itemPriorities[f.prioIdx])))
		b.WriteString(m.detailInput.View())
		b.WriteString(footerStyle.Render("   enter next · esc cancel"))
	case stepDue:
		b.WriteString(detailPromptStyle.Render(fmt.Sprintf("[%s p%d]  due (optional): ", itemTypes[f.typeIdx], itemPriorities[f.prioIdx])))
		b.WriteString(m.detailInput.View())
		b.WriteString(footerStyle.Render("   enter add · esc cancel"))
	}
	return b.String()
}

// stageFoot renders the 's' stage dropdown: the known stages as a vertical
// list with the highlighted choice marked, mirroring the add form's enum
// dropdowns. The current stage is annotated so it's clear where the account is
// moving from.
func (m Model) stageFoot() string {
	var b strings.Builder
	if m.detailStatus != "" {
		b.WriteString(detailStatusStyle.Render(m.detailStatus) + "  ")
	}
	b.WriteString(detailPromptStyle.Render("stage ▾  ") +
		footerStyle.Render("from "+pretty(m.detail.c.stage)+"   ↑/↓ choose · enter advance · esc cancel") + "\n")
	labels := make([]string, len(knownStages))
	for i, s := range knownStages {
		labels[i] = pretty(s)
	}
	b.WriteString(renderDropdown(labels, m.stageIdx))
	return b.String()
}

// renderDropdown renders a vertical option list (a simple dropdown): the
// selected row gets a ▸ marker and the selection background, the rest are faint.
func renderDropdown(opts []string, sel int) string {
	lines := make([]string, len(opts))
	for i, o := range opts {
		if i == sel {
			lines[i] = itemSelStyle.Render(" ▸ " + o + " ")
		} else {
			lines[i] = emptyStyle.Render("   " + o + " ")
		}
	}
	return strings.Join(lines, "\n")
}
