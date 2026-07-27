// Package store drives a Dolt repo by shelling out to the `dolt` CLI. The SQL is
// identical to what an embedded dolthub/driver connection would run, so this can
// be swapped for the in-process driver later without touching command code.
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Store struct {
	Dir string
}

func New(dir string) *Store {
	return &Store{Dir: dir}
}

func (s *Store) run(args ...string) (string, string, error) {
	cmd := exec.Command("dolt", args...)
	cmd.Dir = s.Dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

// EnsureInit makes sure Dir exists and is an initialized Dolt repo, running
// `dolt init` when it is not (so `cs init` on a fresh default home creates the
// repo instead of erroring). It is a no-op when Dir already has a .dolt
// directory, so it is safe to call on every init.
func (s *Store) EnsureInit() error {
	if info, err := os.Stat(filepath.Join(s.Dir, ".dolt")); err == nil && info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create repo dir %s: %w", s.Dir, err)
	}
	if _, errOut, err := s.run("init"); err != nil {
		return fmt.Errorf("dolt init: %v: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

// Exec runs a single SQL statement, expecting no result set.
func (s *Store) Exec(query string) error {
	_, errOut, err := s.run("sql", "-q", query)
	if err != nil {
		return fmt.Errorf("dolt sql: %v: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

// ExecScript pipes a multi-statement SQL script via stdin.
func (s *Store) ExecScript(script string) error {
	cmd := exec.Command("dolt", "sql")
	cmd.Dir = s.Dir
	cmd.Stdin = strings.NewReader(script)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dolt sql (script): %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
}

// Query runs a SELECT and returns rows as maps, using dolt's JSON result format.
func (s *Store) Query(query string) ([]map[string]any, error) {
	out, errOut, err := s.run("sql", "-r", "json", "-q", query)
	if err != nil {
		return nil, fmt.Errorf("dolt sql: %v: %s", err, strings.TrimSpace(errOut))
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	var res struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		return nil, fmt.Errorf("parse dolt json: %v (raw: %.120s)", err, out)
	}
	return res.Rows, nil
}

// Commit stages everything and commits. A no-op (nothing changed) is not an error.
func (s *Store) Commit(msg string) error {
	if _, errOut, err := s.run("add", "-A"); err != nil {
		return fmt.Errorf("dolt add: %v: %s", err, strings.TrimSpace(errOut))
	}
	_, errOut, err := s.run("commit", "-m", msg)
	if err != nil {
		low := strings.ToLower(errOut)
		if strings.Contains(low, "nothing to commit") || strings.Contains(low, "no changes") {
			return nil
		}
		return fmt.Errorf("dolt commit: %v: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}
