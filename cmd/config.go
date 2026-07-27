package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// csHome is the directory cs keeps its own state in (the config pointer, and the
// default data repo when no other location is configured). It lives at ~/.cs so
// cs works from any directory without an env var or a cwd guess.
func csHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cs"), nil
}

// configPath is the file under ~/.cs that stores the data-repo location, so a
// repo living anywhere on disk is remembered without moving it. A single line:
// the absolute path to the Dolt repo.
func configPath() (string, error) {
	h, err := csHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config"), nil
}

// configuredRepo returns the repo path recorded in ~/.cs/config, or "" if the
// file is absent or empty. Read errors other than "not found" are surfaced.
func configuredRepo() (string, error) {
	p, err := configPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	repo := strings.TrimSpace(string(data))
	if repo == "" {
		return "", nil
	}
	// Normalize a hand-edited relative path to absolute so the resolved repo
	// never depends on cwd — that dependence is exactly what this change removes.
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	return repo, nil
}

// writeConfiguredRepo records repo as the data-repo location in ~/.cs/config,
// creating ~/.cs if needed. The path is stored absolute so it resolves the same
// from any directory.
func writeConfiguredRepo(repo string) error {
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	h, err := csHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(h, 0o755); err != nil {
		return err
	}
	p, err := configPath()
	if err != nil {
		return err
	}
	return os.WriteFile(p, []byte(repo+"\n"), 0o644)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or set cs configuration (the data-repo location)",
}

var configSetRepoCmd = &cobra.Command{
	Use:   "set-repo <path>",
	Short: "Record the data-repo location in ~/.cs/config so cs finds it from any directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := writeConfiguredRepo(args[0]); err != nil {
			return err
		}
		repo, _ := configuredRepo()
		fmt.Printf("✓ cs will use %s\n", repo)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the resolved data-repo location and where it came from",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, src := resolveRepoDirWithSource()
		fmt.Printf("repo:   %s\nsource: %s\n", dir, src)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetRepoCmd, configShowCmd)
	rootCmd.AddCommand(configCmd)
}
