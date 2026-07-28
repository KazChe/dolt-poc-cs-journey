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

// shellBackend drives Dolt by shelling out to the `dolt` CLI, one process spawn
// per operation. This is the original, default implementation.
type shellBackend struct {
	dir string
}

func newShellBackend(dir string) *shellBackend {
	return &shellBackend{dir: dir}
}

func (s *shellBackend) run(args ...string) (string, string, error) {
	cmd := exec.Command("dolt", args...)
	cmd.Dir = s.dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func (s *shellBackend) ensureInit() error {
	if info, err := os.Stat(filepath.Join(s.dir, ".dolt")); err == nil && info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create repo dir %s: %w", s.dir, err)
	}
	if _, errOut, err := s.run("init"); err != nil {
		return fmt.Errorf("dolt init: %v: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

func (s *shellBackend) exec(query string) error {
	_, errOut, err := s.run("sql", "-q", query)
	if err != nil {
		return fmt.Errorf("dolt sql: %v: %s", err, strings.TrimSpace(errOut))
	}
	return nil
}

func (s *shellBackend) execScript(script string) error {
	cmd := exec.Command("dolt", "sql")
	cmd.Dir = s.dir
	cmd.Stdin = strings.NewReader(script)
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dolt sql (script): %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return nil
}

func (s *shellBackend) query(query string) ([]map[string]any, error) {
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

func (s *shellBackend) commit(msg string) error {
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
