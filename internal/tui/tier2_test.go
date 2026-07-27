package tui

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/KazChe/cs/internal/store"
)

// setupRepo builds a throwaway dolt repo with one customer and one open item.
func setupRepo(t *testing.T) (*store.Store, string) {
	t.Helper()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("dolt", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("dolt %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	// Dolt needs an author identity to commit; set a repo-local one so the tests
	// don't depend on (or fail without) a global dolt config on the machine/CI.
	run("config", "--local", "--add", "user.name", "cs-test")
	run("config", "--local", "--add", "user.email", "cs-test@example.com")
	st := store.New(dir)
	if err := st.ExecScript(
		"CREATE TABLE customers (id VARCHAR(64) PRIMARY KEY, name VARCHAR(255), stage VARCHAR(32), health VARCHAR(16), updated_at TIMESTAMP);\n" +
			"CREATE TABLE items (id VARCHAR(64) PRIMARY KEY, customer_id VARCHAR(64), type VARCHAR(32), priority INT, title VARCHAR(255), status VARCHAR(32) DEFAULT 'open', due_at DATE NULL, created_at TIMESTAMP, resolved_at TIMESTAMP NULL);\n" +
			"CREATE TABLE activities (id VARCHAR(64) PRIMARY KEY, customer_id VARCHAR(64), kind VARCHAR(32), summary VARCHAR(255), occurred_at TIMESTAMP);\n" +
			"CREATE TABLE stage_events (id VARCHAR(64) PRIMARY KEY, customer_id VARCHAR(64), from_stage VARCHAR(32), to_stage VARCHAR(32), reason VARCHAR(255), occurred_at TIMESTAMP);\n" +
			"INSERT INTO customers VALUES ('acme','Acme','adoption','green',NOW());\n" +
			"INSERT INTO items (id,customer_id,type,priority,title,status,created_at) VALUES ('itm-1','acme','bug',1,'boom','open',NOW());\n"); err != nil {
		t.Fatal(err)
	}
	// Commit the seed so a later action's commit is a distinct, assertable commit
	// rather than sweeping the schema+seed in with it.
	if err := st.Commit("seed"); err != nil {
		t.Fatal(err)
	}
	return st, dir
}

// lastCommitMsg returns the subject of the repo's most recent dolt commit.
func lastCommitMsg(t *testing.T, dir string) string {
	t.Helper()
	c := exec.Command("dolt", "log", "-n", "1", "--oneline")
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("dolt log: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func newDetailModel(st *store.Store) Model {
	m := New(st)
	m.width, m.height = 100, 30
	m.mode = modeDetail
	m.detailVP.Width, m.detailVP.Height = 100, 26
	// load detail synchronously
	msg := loadDetail(st, customer{id: "acme", name: "Acme", stage: "adoption", health: "green"})()
	dm := msg.(detailMsg)
	m.detail = dm.d
	m.syncDetail()
	return m
}

func openCount(t *testing.T, st *store.Store) int {
	rows, err := st.Query("SELECT COUNT(*) AS n FROM items WHERE customer_id='acme' AND status<>'resolved'")
	if err != nil {
		t.Fatal(err)
	}
	return toInt(rows[0]["n"])
}

func TestResolveSelectedCommits(t *testing.T) {
	st, dir := setupRepo(t)
	m := newDetailModel(st)
	if openCount(t, st) != 1 {
		t.Fatalf("precondition: want 1 open item")
	}
	next, _ := m.resolveSelected()
	nm := next.(Model)
	if !strings.Contains(nm.detailStatus, "resolved itm-1") {
		t.Errorf("status = %q, want resolved", nm.detailStatus)
	}
	if openCount(t, st) != 0 {
		t.Errorf("item not resolved in db")
	}
	// The resolve must be its own commit (seed was already committed), so the
	// HEAD commit subject reflects the resolve specifically.
	if msg := lastCommitMsg(t, dir); !strings.Contains(msg, "item: resolve itm-1") {
		t.Errorf("HEAD commit = %q, want the resolve commit", msg)
	}
}

func TestCommitDueSetAndClear(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)

	// set a valid due date
	m.editKind = editDue
	m.detailInput.SetValue("2026-09-01")
	next, _ := m.commitAction()
	m = next.(Model)
	if m.editKind != editNone {
		t.Errorf("editKind should be editNone after commit")
	}
	rows, _ := st.Query("SELECT due_at FROM items WHERE id='itm-1'")
	if got := str(rows[0]["due_at"]); !strings.HasPrefix(got, "2026-09-01") {
		t.Errorf("due_at = %q, want 2026-09-01", got)
	}

	// clear it
	m.editKind = editDue
	m.detailInput.SetValue("")
	next, _ = m.commitAction()
	m = next.(Model)
	rows, _ = st.Query("SELECT due_at FROM items WHERE id='itm-1'")
	if got := rows[0]["due_at"]; got != nil {
		t.Errorf("due_at = %v, want NULL", got)
	}
}

func TestCommitDueRejectsBadDate(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	m.editKind = editDue
	m.detailInput.SetValue("soon")
	next, _ := m.commitAction()
	m = next.(Model)
	if m.editKind != editDue {
		t.Errorf("bad date should keep input open (editKind=editDue)")
	}
	if !strings.Contains(m.detailStatus, "invalid date") {
		t.Errorf("status = %q, want invalid date", m.detailStatus)
	}
}

// keyMsg builds a tea.KeyMsg for a named key (e.g. "enter", "left", "1").
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// sendKeys walks the add form through a sequence of keys via updateAddForm,
// exercising the real step logic. Text steps: set the input value then pass
// "enter" (typing char-by-char through textinput is covered separately).
func sendKeys(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.updateAddForm(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func TestAddItemFormDefaults(t *testing.T) {
	st, dir := setupRepo(t)
	m := newDetailModel(st)
	nb, _ := m.beginEdit(editAdd) // starts the form (type=action, priority=2)
	m = nb.(Model)
	if m.editKind != editAdd || m.addForm.step != stepType {
		t.Fatalf("form did not start on type step")
	}
	// accept defaults: enter (type) → enter (priority) → title → enter → no due → enter
	m = sendKeys(m, "enter", "enter")
	m.detailInput.SetValue("new blocker")
	m = sendKeys(m, "enter") // title → due step
	m = sendKeys(m, "enter") // empty due → commit
	if m.editKind != editNone {
		t.Errorf("editKind should reset after add; status=%q", m.detailStatus)
	}
	rows, _ := st.Query("SELECT type,priority,status,due_at FROM items WHERE customer_id='acme' AND title='new blocker'")
	if len(rows) != 1 {
		t.Fatalf("want 1 new item, got %d", len(rows))
	}
	if got := str(rows[0]["type"]); got != "action" {
		t.Errorf("type = %q, want action (default)", got)
	}
	if got := asIntTest(rows[0]["priority"]); got != 2 {
		t.Errorf("priority = %d, want 2 (default)", got)
	}
	if rows[0]["due_at"] != nil {
		t.Errorf("due_at = %v, want NULL (empty due)", rows[0]["due_at"])
	}
	if msg := lastCommitMsg(t, dir); !strings.Contains(msg, "new blocker") {
		t.Errorf("HEAD commit = %q, want the add commit", msg)
	}
}

func TestAddItemFormChoosesTypePriorityDue(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	nb, _ := m.beginEdit(editAdd)
	m = nb.(Model)
	// type: default idx 3 (action); left once → question (idx 2)
	m = sendKeys(m, "left", "enter")
	// priority: default idx 1 (p2); press "1" → p1
	m = sendKeys(m, "1", "enter")
	m.detailInput.SetValue("SSO bug")
	m = sendKeys(m, "enter") // → due step
	m.detailInput.SetValue("2026-09-15")
	m = sendKeys(m, "enter") // commit
	rows, _ := st.Query("SELECT type,priority,due_at FROM items WHERE customer_id='acme' AND title='SSO bug'")
	if len(rows) != 1 {
		t.Fatalf("want 1 item, got %d", len(rows))
	}
	if got := str(rows[0]["type"]); got != "question" {
		t.Errorf("type = %q, want question", got)
	}
	if got := asIntTest(rows[0]["priority"]); got != 1 {
		t.Errorf("priority = %d, want 1", got)
	}
	if got := str(rows[0]["due_at"]); !strings.HasPrefix(got, "2026-09-15") {
		t.Errorf("due_at = %q, want 2026-09-15", got)
	}
}

func TestAddItemRejectsEmptyTitle(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	nb, _ := m.beginEdit(editAdd)
	m = nb.(Model)
	m = sendKeys(m, "enter", "enter") // to title step
	m.detailInput.SetValue("   ")
	m = sendKeys(m, "enter") // empty title
	if m.addForm.step != stepTitle {
		t.Errorf("empty title should stay on title step, got step %d", m.addForm.step)
	}
	if !strings.Contains(m.detailStatus, "title is required") {
		t.Errorf("status = %q, want title required", m.detailStatus)
	}
}

func TestNoteCommits(t *testing.T) {
	st, dir := setupRepo(t)
	m := newDetailModel(st)
	nb, _ := m.beginEdit(editNote)
	m = nb.(Model)
	m.detailInput.SetValue("called about renewal")
	next, _ := m.commitAction()
	m = next.(Model)
	rows, _ := st.Query("SELECT kind,summary FROM activities WHERE customer_id='acme'")
	if len(rows) != 1 || str(rows[0]["kind"]) != "note" || str(rows[0]["summary"]) != "called about renewal" {
		t.Fatalf("note not recorded correctly: %v", rows)
	}
	if msg := lastCommitMsg(t, dir); !strings.Contains(msg, "note:") {
		t.Errorf("HEAD commit = %q, want the note commit", msg)
	}
}

// TestNoteEscapesQuotes locks in that user text with a single quote is stored
// verbatim (not broken or truncated), i.e. q() escaping holds on the write path.
func TestNoteEscapesQuotes(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	const summary = "O'Brien said it's fine"
	nb, _ := m.beginEdit(editNote)
	m = nb.(Model)
	m.detailInput.SetValue(summary)
	next, _ := m.commitAction()
	m = next.(Model)
	if m.editKind != editNone {
		t.Errorf("commit should have closed the input; status=%q", m.detailStatus)
	}
	rows, _ := st.Query("SELECT summary FROM activities WHERE customer_id='acme'")
	if len(rows) != 1 || str(rows[0]["summary"]) != summary {
		t.Fatalf("summary round-trip failed: %v", rows)
	}
}

func TestStageAdvanceCommits(t *testing.T) {
	st, dir := setupRepo(t)
	m := newDetailModel(st)
	// account starts in 'adoption' (see setupRepo)
	nb, _ := m.beginEdit(editStage)
	m = nb.(Model)
	m.detailInput.SetValue("expansion")
	next, _ := m.commitAction()
	m = next.(Model)
	if m.detail.c.stage != "expansion" {
		t.Errorf("header stage = %q, want expansion", m.detail.c.stage)
	}
	rows, _ := st.Query("SELECT stage FROM customers WHERE id='acme'")
	if got := str(rows[0]["stage"]); got != "expansion" {
		t.Errorf("customer stage = %q, want expansion", got)
	}
	ev, _ := st.Query("SELECT from_stage,to_stage FROM stage_events WHERE customer_id='acme'")
	if len(ev) != 1 || str(ev[0]["from_stage"]) != "adoption" || str(ev[0]["to_stage"]) != "expansion" {
		t.Fatalf("stage_event not recorded: %v", ev)
	}
	if msg := lastCommitMsg(t, dir); !strings.Contains(msg, "adoption->expansion") {
		t.Errorf("HEAD commit = %q, want the stage commit", msg)
	}
}

func TestStageAdvanceSetsHealth(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	// account starts in 'adoption'/'green'; renewal_risk should flip health to red.
	nb, _ := m.beginEdit(editStage)
	m = nb.(Model)
	m.detailInput.SetValue("renewal_risk")
	next, _ := m.commitAction()
	m = next.(Model)
	if m.detail.c.health != "red" {
		t.Errorf("header health = %q, want red", m.detail.c.health)
	}
	rows, _ := st.Query("SELECT stage,health FROM customers WHERE id='acme'")
	if got := str(rows[0]["stage"]); got != "renewal_risk" {
		t.Errorf("customer stage = %q, want renewal_risk", got)
	}
	if got := str(rows[0]["health"]); got != "red" {
		t.Errorf("customer health = %q, want red", got)
	}
}

func TestHealthForStage(t *testing.T) {
	want := map[string]string{
		"onboarding":   "yellow",
		"adoption":     "green",
		"expansion":    "green",
		"renewal_risk": "red",
		"renewed":      "green",
	}
	for stage, exp := range want {
		if got := healthForStage(stage); got != exp {
			t.Errorf("healthForStage(%q) = %q, want %q", stage, got, exp)
		}
	}
	if got := healthForStage("unknown"); got != "yellow" {
		t.Errorf("healthForStage(unknown) = %q, want yellow fallback", got)
	}
}

func TestStageRejectsUnknown(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	nb, _ := m.beginEdit(editStage)
	m = nb.(Model)
	m.detailInput.SetValue("banana")
	next, _ := m.commitAction()
	m = next.(Model)
	if m.editKind != editStage {
		t.Errorf("unknown stage should keep input open")
	}
	if !strings.Contains(m.detailStatus, "unknown stage") {
		t.Errorf("status = %q, want unknown stage", m.detailStatus)
	}
}

// asIntTest coerces a dolt JSON numeric to int for assertions.
func asIntTest(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return toInt(v)
}
