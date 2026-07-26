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

		// On-demand due block: surfaces overdue/upcoming items so opening a
		// Claude Code session (SessionStart hook) or running `cs prime` in a
		// terminal both flag time-sensitive work. Omitted entirely when nothing
		// is due, to keep the snapshot quiet.
		due, err := dueItems(st, defaultDueWindow())
		if err != nil {
			return err
		}
		if overdue, upcoming := splitDue(due); len(due) > 0 {
			b.WriteString("\n## Due soon / overdue\n")
			for _, r := range overdue {
				b.WriteString(fmt.Sprintf("- ⚠ overdue: %v (%v) %v — %v\n",
					r["title"], r["customer_id"], dueDateStr(r["due_at"]), r["type"]))
			}
			for _, r := range upcoming {
				b.WriteString(fmt.Sprintf("- due %v: %v (%v) — %v\n",
					dueDateStr(r["due_at"]), r["title"], r["customer_id"], r["type"]))
			}
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
