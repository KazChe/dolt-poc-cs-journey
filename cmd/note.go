package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/ui"
)

var (
	noteCust   string
	noteKind   string
	noteCommit bool
)

var noteCmd = &cobra.Command{
	Use:   "note [summary...]",
	Short: "Append an activity to a customer's journal",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()

		custID := noteCust
		if custID == "" {
			picked, err := ui.PickCustomer(st)
			if err != nil {
				return err
			}
			custID = picked
		}

		summary := strings.TrimSpace(strings.Join(args, " "))
		if summary == "" {
			// No summary on the command line: prompt for it, so `cs note` (or
			// after the fuzzy picker) lets you type the note inline.
			entered, err := promptLine(fmt.Sprintf("Note for %s: ", custID))
			if err != nil {
				return err
			}
			summary = strings.TrimSpace(entered)
		}
		if summary == "" {
			return fmt.Errorf("a note summary is required")
		}

		id := newID("act")
		q := fmt.Sprintf(
			"INSERT INTO activities (id,customer_id,kind,summary,occurred_at) VALUES (%s,%s,%s,%s,NOW())",
			sqlStr(id), sqlStr(custID), sqlStr(noteKind), sqlStr(summary))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ noted on %s: %s\n", custID, summary)

		if noteCommit {
			if err := st.Commit(fmt.Sprintf("note: %s - %s", custID, summary)); err != nil {
				return err
			}
			fmt.Println("✓ committed")
		}
		return nil
	},
}

func init() {
	noteCmd.Flags().StringVarP(&noteCust, "cust", "c", "", "customer id (omit to fuzzy-pick)")
	noteCmd.Flags().StringVarP(&noteKind, "kind", "k", "note", "activity kind: call|slack|email|ticket|meeting|note")
	noteCmd.Flags().BoolVar(&noteCommit, "commit", false, "commit this change immediately")
	rootCmd.AddCommand(noteCmd)
}
