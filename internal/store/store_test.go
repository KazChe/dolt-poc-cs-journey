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

// backends lists the backends to test in this build. It always includes
// shellout; the embedded backend is appended only in embedded-tagged builds
// (see backends_embedded_test.go), since a default build has no embedded driver
// compiled in. Every behavior test runs against all listed backends via
// forEachBackend — the parity net for issue #4.
var backends = append([]string{backendShellout}, extraBackends...)

// newTestStore returns a Store backed by a fresh, initialized Dolt repo in a
// t.TempDir(), using the given backend. It skips the test if the dolt binary is
// not on PATH, so the suite degrades gracefully in environments without Dolt.
func newTestStore(t testing.TB, backend string) *Store {
	t.Helper()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt binary not on PATH; skipping store test")
	}
	t.Setenv(EnvBackend, backend)
	s := New(t.TempDir())
	if s.Backend() != backend {
		t.Fatalf("Backend() = %q, want %q", s.Backend(), backend)
	}
	if err := s.EnsureInit(); err != nil {
		t.Fatalf("EnsureInit: %v", err)
	}
	return s
}

// forEachBackend runs fn as a subtest for every backend, giving each a fresh
// Store. Behavior tests wrap their body in this so a single test asserts parity
// across both implementations.
func forEachBackend(t *testing.T, fn func(t *testing.T, s *Store)) {
	t.Helper()
	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			fn(t, newTestStore(t, backend))
		})
	}
}

func TestEnsureInitIdempotent(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
		// A second EnsureInit on an already-initialized repo must be a no-op, not
		// an error (cs init calls it on every invocation).
		if err := s.EnsureInit(); err != nil {
			t.Fatalf("second EnsureInit: %v", err)
		}
	})
}

func TestExecScriptAndQueryRoundTrip(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
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
	})
}

func TestQueryEmptyResult(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
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
	})
}

func TestExecError(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
		// Inserting into a non-existent table must surface an error, not swallow it.
		if err := s.Exec(`INSERT INTO does_not_exist (x) VALUES (1)`); err == nil {
			t.Fatal("expected error inserting into missing table, got nil")
		}
	})
}

func TestCommitAndHistory(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
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
	})
}

func TestCommitNothingToCommit(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s *Store) {
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
	})
}

// --- Benchmarks: shell-out vs embedded per-op latency (issue #4).
// Run both to quantify the difference. The shell-out path pays a full `dolt`
// process spawn per op; the embedded driver amortizes startup across a
// long-lived connection.

func benchStore(b *testing.B, backend string) *Store {
	s := newTestStore(b, backend)
	if err := s.ExecScript(testSchema); err != nil {
		b.Fatalf("ExecScript: %v", err)
	}
	return s
}

func BenchmarkExec(b *testing.B) {
	for _, backend := range backends {
		b.Run(backend, func(b *testing.B) {
			s := benchStore(b, backend)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				q := fmt.Sprintf(`INSERT INTO customers (id, name) VALUES ('c%d', 'Cust %d')`, i, i)
				if err := s.Exec(q); err != nil {
					b.Fatalf("Exec: %v", err)
				}
			}
		})
	}
}

func BenchmarkQuery(b *testing.B) {
	for _, backend := range backends {
		b.Run(backend, func(b *testing.B) {
			s := benchStore(b, backend)
			if err := s.Exec(`INSERT INTO customers (id, name, stage) VALUES ('acme', 'Acme Inc', 'live')`); err != nil {
				b.Fatalf("seed: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Query(`SELECT id, name, stage FROM customers WHERE id='acme'`); err != nil {
					b.Fatalf("Query: %v", err)
				}
			}
		})
	}
}
