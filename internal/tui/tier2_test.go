package tui

import (
	"os/exec"
	"strings"
	"testing"

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
	m.dueEditing = true
	m.detailInput.SetValue("2026-09-01")
	next, _ := m.commitDue()
	m = next.(Model)
	if m.dueEditing {
		t.Errorf("dueEditing should be false after commit")
	}
	rows, _ := st.Query("SELECT due_at FROM items WHERE id='itm-1'")
	if got := str(rows[0]["due_at"]); !strings.HasPrefix(got, "2026-09-01") {
		t.Errorf("due_at = %q, want 2026-09-01", got)
	}

	// clear it
	m.dueEditing = true
	m.detailInput.SetValue("")
	next, _ = m.commitDue()
	m = next.(Model)
	rows, _ = st.Query("SELECT due_at FROM items WHERE id='itm-1'")
	if got := rows[0]["due_at"]; got != nil {
		t.Errorf("due_at = %v, want NULL", got)
	}
}

func TestCommitDueRejectsBadDate(t *testing.T) {
	st, _ := setupRepo(t)
	m := newDetailModel(st)
	m.dueEditing = true
	m.detailInput.SetValue("soon")
	next, _ := m.commitDue()
	m = next.(Model)
	if !m.dueEditing {
		t.Errorf("bad date should keep input open (dueEditing=true)")
	}
	if !strings.Contains(m.detailStatus, "invalid date") {
		t.Errorf("status = %q, want invalid date", m.detailStatus)
	}
}
