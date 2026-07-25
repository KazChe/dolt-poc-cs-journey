package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var primeHookJSON bool

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Print a bounded snapshot of all accounts (for a SessionStart hook)",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		rows, err := st.Query(
			"SELECT c.id, c.name, c.stage, c.health, " +
				"(SELECT COUNT(*) FROM items i WHERE i.customer_id=c.id AND i.status<>'resolved') AS open_items " +
				"FROM customers c ORDER BY c.name")
		if err != nil {
			return err
		}

		var b strings.Builder
		b.WriteString("# Customer snapshot\n")
		for _, r := range rows {
			b.WriteString(fmt.Sprintf("- %v (%v): stage=%v health=%v open=%v\n",
				r["name"], r["id"], r["stage"], r["health"], r["open_items"]))
		}
		content := b.String()

		if primeHookJSON {
			env := map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":     "SessionStart",
					"additionalContext": content,
				},
			}
			return json.NewEncoder(os.Stdout).Encode(env)
		}
		fmt.Print(content)
		return nil
	},
}

func init() {
	primeCmd.Flags().BoolVar(&primeHookJSON, "hook-json", false, "wrap output in the Claude Code SessionStart envelope")
	rootCmd.AddCommand(primeCmd)
}
