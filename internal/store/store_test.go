package store

import (
	"fmt"
	"os/exec"
	"testing"
)

// The store schema mirrors cmd/schema.sql closely enough to exercise every
// store method. It is kept inline (rather than embedding the real schema file)
// so the store package stays free of a dependency on cmd/.
const testSchema = `
CREATE TABLE IF NOT EXISTS customers (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  stage VARCHAR(32) NOT NULL DEFAULT 'onboarding',
  health VARCHAR(16) DEFAULT 'green',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS items (
  id VARCHAR(64) PRIMARY KEY,
  customer_id VARCHAR(64) NOT NULL,
  type VARCHAR(16) NOT NULL,
  title VARCHAR(255) NOT NULL,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  INDEX (customer_id, status)
);
`

// newTestStore returns a Store backed by a fresh, initialized Dolt repo in a
// t.TempDir(). It skips the test if the dolt binary is not on PATH, so the
// suite degrades gracefully in environments without Dolt installed.
func newTestStore(t testing.TB) *Store {
	t.Helper()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt binary not on PATH; skipping store test")
	}
	s := New(t.TempDir())
	if err := s.EnsureInit(); err != nil {
		t.Fatalf("EnsureInit: %v", err)
	}
	return s
}

func TestEnsureInitIdempotent(t *testing.T) {
	s := newTestStore(t)
	// A second EnsureInit on an already-initialized repo must be a no-op, not an
	// error (cs init calls it on every invocation).
	if err := s.EnsureInit(); err != nil {
		t.Fatalf("second EnsureInit: %v", err)
	}
}

func TestExecScriptAndQueryRoundTrip(t *testing.T) {
	s := newTestStore(t)
	if err := s.ExecScript(testSchema); err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	if err := s.Exec(`INSERT INTO customers (id, name, stage) VALUES ('acme', 'Acme Inc', 'live')`); err != nil {
		t.Fatalf("Exec insert: %v", err)
	}

	rows, err := s.Query(`SELECT id, name, stage FROM customers WHERE id='acme'`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	got := rows[0]
	if got["id"] != "acme" || got["name"] != "Acme Inc" || got["stage"] != "live" {
		t.Fatalf("unexpected row: %+v", got)
	}
}

func TestQueryEmptyResult(t *testing.T) {
	s := newTestStore(t)
	if err := s.ExecScript(testSchema); err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	rows, err := s.Query(`SELECT id FROM customers WHERE id='nope'`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestExecError(t *testing.T) {
	s := newTestStore(t)
	// Querying a non-existent table must surface an error, not swallow it.
	if err := s.Exec(`INSERT INTO does_not_exist (x) VALUES (1)`); err == nil {
		t.Fatal("expected error inserting into missing table, got nil")
	}
}

func TestCommitAndHistory(t *testing.T) {
	s := newTestStore(t)
	if err := s.ExecScript(testSchema); err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	if err := s.Commit("init schema"); err != nil {
		t.Fatalf("Commit schema: %v", err)
	}
	if err := s.Exec(`INSERT INTO customers (id, name) VALUES ('beta', 'Beta LLC')`); err != nil {
		t.Fatalf("Exec insert: %v", err)
	}
	if err := s.Commit("add beta"); err != nil {
		t.Fatalf("Commit beta: %v", err)
	}

	rows, err := s.Query(`SELECT message FROM dolt_log ORDER BY date DESC LIMIT 1`)
	if err != nil {
		t.Fatalf("Query dolt_log: %v", err)
	}
	if len(rows) != 1 || rows[0]["message"] != "add beta" {
		t.Fatalf("expected latest commit 'add beta', got %+v", rows)
	}
}

func TestCommitNothingToCommit(t *testing.T) {
	s := newTestStore(t)
	if err := s.ExecScript(testSchema); err != nil {
		t.Fatalf("ExecScript: %v", err)
	}
	if err := s.Commit("first"); err != nil {
		t.Fatalf("Commit first: %v", err)
	}
	// A second commit with no intervening changes must be a no-op, not an error.
	if err := s.Commit("noop"); err != nil {
		t.Fatalf("Commit noop should be nil, got: %v", err)
	}
}

// --- Benchmarks: baseline for the shell-out -> embedded driver swap (issue #4).
// Run before the swap on the current shell-out impl, then re-run after the swap
// to quantify the per-op latency difference. Each op currently pays a full
// `dolt` process spawn; the embedded driver should amortize startup.

func benchStore(b *testing.B) *Store {
	s := newTestStore(b)
	if err := s.ExecScript(testSchema); err != nil {
		b.Fatalf("ExecScript: %v", err)
	}
	return s
}

func BenchmarkExec(b *testing.B) {
	s := benchStore(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q := fmt.Sprintf(`INSERT INTO customers (id, name) VALUES ('c%d', 'Cust %d')`, i, i)
		if err := s.Exec(q); err != nil {
			b.Fatalf("Exec: %v", err)
		}
	}
}

func BenchmarkQuery(b *testing.B) {
	s := benchStore(b)
	if err := s.Exec(`INSERT INTO customers (id, name, stage) VALUES ('acme', 'Acme Inc', 'live')`); err != nil {
		b.Fatalf("seed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Query(`SELECT id, name, stage FROM customers WHERE id='acme'`); err != nil {
			b.Fatalf("Query: %v", err)
		}
	}
}
