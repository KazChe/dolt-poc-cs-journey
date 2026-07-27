package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveRepoDirPrecedence pins the resolution order: --repo flag beats
// $CS_DIR beats ~/.cs/config beats the ~/.cs default. Each case isolates HOME to
// a temp dir so the real ~/.cs is never touched, and resets the --repo flag var
// and CS_DIR between cases.
func TestResolveRepoDirPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	csDir := filepath.Join(home, ".cs")

	writeConfig := func(path string) {
		if err := os.MkdirAll(csDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(csDir, "config"), []byte(path+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clearConfig := func() { _ = os.Remove(filepath.Join(csDir, "config")) }

	t.Run("default when nothing set", func(t *testing.T) {
		repoDir = ""
		t.Setenv("CS_DIR", "")
		clearConfig()
		dir, src := resolveRepoDirWithSource()
		if dir != csDir {
			t.Errorf("dir = %q, want %q", dir, csDir)
		}
		if src != "default (~/.cs)" {
			t.Errorf("source = %q, want default", src)
		}
	})

	t.Run("whitespace-only config falls through to default", func(t *testing.T) {
		repoDir = ""
		t.Setenv("CS_DIR", "")
		writeConfig("   ")
		defer clearConfig()
		dir, src := resolveRepoDirWithSource()
		if dir != csDir || src != "default (~/.cs)" {
			t.Errorf("got (%q, %q), want (%q, default)", dir, src, csDir)
		}
	})

	t.Run("config beats default", func(t *testing.T) {
		repoDir = ""
		t.Setenv("CS_DIR", "")
		writeConfig("/data/from-config")
		defer clearConfig()
		dir, src := resolveRepoDirWithSource()
		if dir != "/data/from-config" {
			t.Errorf("dir = %q, want /data/from-config", dir)
		}
		if src != "~/.cs/config" {
			t.Errorf("source = %q, want config", src)
		}
	})

	t.Run("CS_DIR beats config", func(t *testing.T) {
		repoDir = ""
		writeConfig("/data/from-config")
		defer clearConfig()
		t.Setenv("CS_DIR", "/data/from-env")
		dir, src := resolveRepoDirWithSource()
		if dir != "/data/from-env" {
			t.Errorf("dir = %q, want /data/from-env", dir)
		}
		if src != "$CS_DIR" {
			t.Errorf("source = %q, want $CS_DIR", src)
		}
	})

	t.Run("--repo beats everything", func(t *testing.T) {
		writeConfig("/data/from-config")
		defer clearConfig()
		t.Setenv("CS_DIR", "/data/from-env")
		repoDir = "/data/from-flag"
		defer func() { repoDir = "" }()
		dir, src := resolveRepoDirWithSource()
		if dir != "/data/from-flag" {
			t.Errorf("dir = %q, want /data/from-flag", dir)
		}
		if src != "--repo flag" {
			t.Errorf("source = %q, want --repo flag", src)
		}
	})
}

// TestWriteAndReadConfiguredRepo round-trips the config pointer and confirms the
// stored path is absolute.
func TestWriteAndReadConfiguredRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, err := configuredRepo(); err != nil || got != "" {
		t.Fatalf("configuredRepo() on empty = %q, %v; want \"\", nil", got, err)
	}
	if err := writeConfiguredRepo("/tmp/some/repo"); err != nil {
		t.Fatal(err)
	}
	got, err := configuredRepo()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/tmp/some/repo" {
		t.Errorf("configuredRepo() = %q, want /tmp/some/repo", got)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("stored path %q is not absolute", got)
	}
}
