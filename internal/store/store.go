// Package store drives a Dolt repo. It has two interchangeable backends behind a
// common surface: the default "shellout" backend spawns the `dolt` CLI per call,
// and the "embedded" backend runs SQL in-process via dolthub/driver/v2. The SQL,
// on-disk .dolt repo, and observable behavior are identical across both, so the
// backend can be switched without touching command code.
//
// The backend is selected by the CS_STORE_BACKEND environment variable:
//
//	CS_STORE_BACKEND=shellout   (default) — spawn the dolt binary per operation
//	CS_STORE_BACKEND=embedded             — in-process dolthub/driver/v2
//
// An unset or unrecognized value falls back to shellout, so existing behavior is
// unchanged until a caller opts in.
package store

import (
	"fmt"
	"os"
)

// Backend names for the CS_STORE_BACKEND environment variable.
const (
	backendShellout = "shellout"
	backendEmbedded = "embedded"

	// EnvBackend is the environment variable that selects the store backend.
	EnvBackend = "CS_STORE_BACKEND"
)

// backend is the internal seam both implementations satisfy. Store delegates
// every public method to the selected backend; the method set mirrors Store's
// public API exactly so the two stay in lockstep.
type backend interface {
	ensureInit() error
	exec(query string) error
	execScript(script string) error
	query(query string) ([]map[string]any, error)
	commit(msg string) error
}

// Store drives a Dolt repo in Dir through the selected backend.
type Store struct {
	Dir     string
	backend string
	be      backend
}

// New returns a Store for the Dolt repo in dir, selecting the backend from
// CS_STORE_BACKEND (default shellout). Selection happens once, here, so the rest
// of the program is backend-agnostic.
func New(dir string) *Store {
	s := &Store{Dir: dir}
	switch os.Getenv(EnvBackend) {
	case backendEmbedded:
		s.backend = backendEmbedded
		s.be = newEmbeddedBackend(dir)
	default: // "", "shellout", or anything unrecognized
		s.backend = backendShellout
		s.be = newShellBackend(dir)
	}
	return s
}

// Backend reports which backend this Store is using ("shellout" or "embedded").
// Handy for tests, benchmarks, and diagnostics.
func (s *Store) Backend() string { return s.backend }

// EnsureInit makes sure Dir exists and is an initialized Dolt repo, running
// `dolt init` when it is not (so `cs init` on a fresh default home creates the
// repo instead of erroring). It is a no-op when Dir already has a .dolt
// directory, so it is safe to call on every init.
func (s *Store) EnsureInit() error {
	if err := s.be.ensureInit(); err != nil {
		return fmt.Errorf("ensure init: %w", err)
	}
	return nil
}

// Exec runs a single SQL statement, expecting no result set.
func (s *Store) Exec(query string) error { return s.be.exec(query) }

// ExecScript runs a multi-statement SQL script.
func (s *Store) ExecScript(script string) error { return s.be.execScript(script) }

// Query runs a SELECT and returns rows as maps.
func (s *Store) Query(query string) ([]map[string]any, error) { return s.be.query(query) }

// Commit stages everything and commits. A no-op (nothing changed) is not an error.
func (s *Store) Commit(msg string) error { return s.be.commit(msg) }
