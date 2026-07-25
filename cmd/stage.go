package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	stageReason string
	stageCommit bool
)

var stageCmd = &cobra.Command{
	Use:   "stage <customer> <to-stage>",
	Short: "Record a stage transition for a customer",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		cust, to := args[0], args[1]

		rows, err := st.Query(fmt.Sprintf("SELECT stage FROM customers WHERE id=%s", sqlStr(cust)))
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("no such customer: %s", cust)
		}
		from := fmt.Sprintf("%v", rows[0]["stage"])

		id := newID("stg")
		script := fmt.Sprintf(
			"INSERT INTO stage_events (id,customer_id,from_stage,to_stage,reason,occurred_at) VALUES (%s,%s,%s,%s,%s,NOW());\n"+
				"UPDATE customers SET stage=%s, updated_at=NOW() WHERE id=%s;\n",
			sqlStr(id), sqlStr(cust), sqlStr(from), sqlStr(to), sqlStr(stageReason),
			sqlStr(to), sqlStr(cust))
		if err := st.ExecScript(script); err != nil {
			return err
		}
		fmt.Printf("✓ %s: %s -> %s\n", cust, from, to)
		return maybeCommit(st, stageCommit, fmt.Sprintf("stage: %s %s->%s", cust, from, to))
	},
}

func init() {
	stageCmd.Flags().StringVar(&stageReason, "reason", "", "why the stage changed")
	stageCmd.Flags().BoolVar(&stageCommit, "commit", false, "commit immediately")
	rootCmd.AddCommand(stageCmd)
}
