//go:build embedded

// This file, and the dolthub/driver/v2 dependency it pulls, are compiled only
// when building with -tags embedded. Default builds use backend_embedded_stub.go
// instead, so the plain `cs` binary stays pure-Go and needs no CGO/ICU toolchain.
// Selecting CS_STORE_BACKEND=embedded at runtime requires an embedded-tagged build.

package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	_ "github.com/dolthub/driver/v2"
)

// embeddedBackend drives Dolt in-process through dolthub/driver/v2, so each
// operation runs against a long-lived *sql.DB rather than spawning the dolt
// binary. The on-disk .dolt repo and SQL are identical to the shell backend.
//
// Repo creation (`dolt init`) is not a driver operation, so ensureInit still
// shells out to `dolt init` for the one-time bootstrap. Every hot-path
// operation (Exec/Query/Commit) goes through the driver.
type embeddedBackend struct {
	dir string

	mu sync.Mutex
	db *sql.DB
}

func newEmbeddedBackend(dir string) backend {
	return &embeddedBackend{dir: dir}
}

// dsn builds the file:// datasource for dir. The driver requires commitname and
// commitemail. database, when set, makes that database the session default on
// every pooled connection — essential because database/sql hands out multiple
// connections and a one-off `USE` would not stick across them.
func (e *embeddedBackend) dsn(database string) string {
	q := url.Values{
		"commitname":      []string{"cs"},
		"commitemail":     []string{"cs@localhost"},
		"multistatements": []string{"true"},
	}
	if database != "" {
		q.Set("database", database)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(e.dir), RawQuery: q.Encode()}
	return u.String()
}

// conn lazily opens the *sql.DB bound to the repo's user database and caches the
// handle. It discovers the database name once (with a database-less handle) so
// it never has to replicate Dolt's directory-to-database naming, then reopens
// with database set in the DSN so every pooled connection defaults to it. Safe
// for concurrent use.
func (e *embeddedBackend) conn() (*sql.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.db != nil {
		return e.db, nil
	}
	discover, err := sql.Open("dolt", e.dsn(""))
	if err != nil {
		return nil, fmt.Errorf("open dolt driver: %w", err)
	}
	name, err := userDatabase(discover)
	discover.Close()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("dolt", e.dsn(name))
	if err != nil {
		return nil, fmt.Errorf("open dolt driver (database %q): %w", name, err)
	}
	e.db = db
	return e.db, nil
}

// userDatabase returns the single non-system database in the repo. Dolt derives
// the database name from the repo directory (with its own sanitization), so we
// discover it rather than guess it.
func userDatabase(db *sql.DB) (string, error) {
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return "", fmt.Errorf("show databases: %w", err)
	}
	defer rows.Close()
	var found string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", fmt.Errorf("scan database name: %w", err)
		}
		switch strings.ToLower(name) {
		case "information_schema", "mysql", "dolt_cluster", "performance_schema":
			continue
		}
		if found != "" {
			return "", fmt.Errorf("expected one user database, found %q and %q", found, name)
		}
		found = name
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no user database found in repo")
	}
	return found, nil
}

func (e *embeddedBackend) ensureInit() error {
	// If the repo already exists, just make sure we can connect.
	if info, err := os.Stat(filepath.Join(e.dir, ".dolt")); err == nil && info.IsDir() {
		_, err := e.conn()
		return err
	}
	// Bootstrap the repo. `dolt init` is a CLI operation, not a driver one.
	if err := os.MkdirAll(e.dir, 0o755); err != nil {
		return fmt.Errorf("create repo dir %s: %w", e.dir, err)
	}
	cmd := exec.Command("dolt", "init")
	cmd.Dir = e.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dolt init: %v: %s", err, strings.TrimSpace(string(out)))
	}
	_, err := e.conn()
	return err
}

func (e *embeddedBackend) exec(query string) error {
	db, err := e.conn()
	if err != nil {
		return err
	}
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("dolt exec: %w", err)
	}
	return nil
}

func (e *embeddedBackend) execScript(script string) error {
	db, err := e.conn()
	if err != nil {
		return err
	}
	// multistatements=true in the DSN lets a single Exec run the whole script.
	if _, err := db.Exec(script); err != nil {
		return fmt.Errorf("dolt exec (script): %w", err)
	}
	return nil
}

func (e *embeddedBackend) query(query string) ([]map[string]any, error) {
	db, err := e.conn()
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("dolt query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeValue(vals[i])
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// normalizeValue coerces driver-returned values to the same shapes the shell
// backend's JSON decoder produces, so callers see identical results across
// backends. The dolt JSON output renders every scalar as a string, so []byte
// and string both become string here.
func normalizeValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return v
	}
}

func (e *embeddedBackend) commit(msg string) error {
	db, err := e.conn()
	if err != nil {
		return err
	}
	if _, err := db.Exec("CALL DOLT_ADD('-A')"); err != nil {
		return fmt.Errorf("dolt add: %w", err)
	}
	if _, err := db.Exec("CALL DOLT_COMMIT('-m', ?)", msg); err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "nothing to commit") || strings.Contains(low, "no changes") {
			return nil
		}
		return fmt.Errorf("dolt commit: %w", err)
	}
	return nil
}
