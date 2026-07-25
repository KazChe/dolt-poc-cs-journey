package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/ui"
)

var (
	linkRel    string
	linkCommit bool
)

var linkCmd = &cobra.Command{
	Use:   "link [from-id] [to-id]",
	Short: "Add a relationship edge between two items (fuzzy-pick if omitted)",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		var from, to string
		var err error
		if len(args) >= 2 {
			from, to = args[0], args[1]
		} else {
			if from, err = ui.PickItem(st, "1=1"); err != nil {
				return err
			}
			if to, err = ui.PickItem(st, fmt.Sprintf("id <> %s", sqlStr(from))); err != nil {
				return err
			}
		}
		q := fmt.Sprintf(
			"INSERT INTO edges (from_id,to_id,rel) VALUES (%s,%s,%s)",
			sqlStr(from), sqlStr(to), sqlStr(linkRel))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ %s -%s-> %s\n", from, linkRel, to)
		return maybeCommit(st, linkCommit, fmt.Sprintf("link: %s %s %s", from, linkRel, to))
	},
}

func init() {
	linkCmd.Flags().StringVar(&linkRel, "rel", "blocks", "relationship: blocks|relates|raised_in|advances_stage|supersedes")
	linkCmd.Flags().BoolVar(&linkCommit, "commit", false, "commit immediately")
	rootCmd.AddCommand(linkCmd)
}
