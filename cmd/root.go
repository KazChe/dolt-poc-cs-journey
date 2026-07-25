package cmd

import (
	"fmt"
	"os"

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
	rootCmd.PersistentFlags().StringVar(&repoDir, "repo", "", "path to the Dolt repo (default $CS_DIR, then cwd)")
}

// mustStore resolves the Dolt repo directory from --repo, $CS_DIR, or cwd.
func mustStore() *store.Store {
	dir := repoDir
	if dir == "" {
		dir = os.Getenv("CS_DIR")
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return store.New(dir)
}
