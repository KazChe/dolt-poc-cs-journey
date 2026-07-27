package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/store"
)

var repoDir string

var rootCmd = &cobra.Command{
	Use:   "cs",
	Short: "Customer-success journey tracker, versioned on Dolt",
	Long: "cs records customer activities, stateful asks, relationships, and\n" +
		"stage transitions in a Dolt repo, so a customer's journey is queryable\n" +
		"through time (diff, as-of) without extra modeling.",
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&repoDir, "repo", "", "path to the Dolt repo (default $CS_DIR, then ~/.cs/config, then ~/.cs)")
}

// resolveRepoDir resolves the Dolt repo directory. Precedence, most to least
// specific: the --repo flag, the $CS_DIR env var, the path recorded in
// ~/.cs/config, and finally the ~/.cs default. It intentionally does NOT fall
// back to the current working directory: that made the resolved repo depend on
// which folder you happened to run from, which silently pointed cs at the wrong
// (or a non-cs) repo. See resolveRepoDirWithSource for the same logic with a
// human-readable source label.
func resolveRepoDir() string {
	dir, _ := resolveRepoDirWithSource()
	return dir
}

// resolveRepoDirWithSource is resolveRepoDir plus a label describing which input
// won, for `cs config show` and clearer error messages.
func resolveRepoDirWithSource() (dir, source string) {
	if repoDir != "" {
		return repoDir, "--repo flag"
	}
	if env := os.Getenv("CS_DIR"); env != "" {
		return env, "$CS_DIR"
	}
	if cfg, err := configuredRepo(); err == nil && cfg != "" {
		return cfg, "~/.cs/config"
	}
	if h, err := csHome(); err == nil {
		return h, "default (~/.cs)"
	}
	// Only if the home dir is somehow unavailable.
	return ".cs", "fallback"
}

// mustStore resolves the Dolt repo directory and returns a store for it. It
// fails fast with an actionable message when the resolved directory is not a
// Dolt repo yet, rather than letting a raw `dolt sql` error surface later.
func mustStore() *store.Store {
	dir, source := resolveRepoDirWithSource()
	if !isDoltRepo(dir) {
		fmt.Fprintf(os.Stderr,
			"cs: no Dolt repo at %s (%s).\n"+
				"Run `cs init` to create one there, or point cs at an existing repo with\n"+
				"`cs config set-repo <path>` (or --repo <path> / CS_DIR=<path>).\n",
			dir, source)
		os.Exit(1)
	}
	return store.New(dir)
}

// isDoltRepo reports whether dir looks like an initialized Dolt repo (has a
// .dolt directory). Used to give a friendly error before shelling out to dolt.
func isDoltRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".dolt"))
	return err == nil && info.IsDir()
}
