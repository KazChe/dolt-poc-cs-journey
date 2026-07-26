package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show [customer]",
	Short: "Show a customer's state, open items, recent activity, and trajectory",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		var cust string
		var err error
		if len(args) > 0 {
			cust = args[0]
		} else if cust, err = resolveCustomer(st, ""); err != nil {
			return err
		}

		head, err := st.Query(fmt.Sprintf("SELECT name,stage,health FROM customers WHERE id=%s", sqlStr(cust)))
		if err != nil {
			return err
		}
		if len(head) == 0 {
			return fmt.Errorf("no such customer: %s", cust)
		}
		h := head[0]
		fmt.Printf("%v (%s)  stage=%v  health=%v\n", h["name"], cust, h["stage"], h["health"])

		fmt.Println("\nOpen items (oldest first):")
		items, err := st.Query(fmt.Sprintf(
			"SELECT id,type,priority,title,created_at,due_at FROM items WHERE customer_id=%s AND status<>'resolved' ORDER BY created_at",
			sqlStr(cust)))
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("  (none)")
		}
		for _, r := range items {
			meta := fmt.Sprintf("%s old", ageDays(r["created_at"]))
			if due := dueAnnotation(r["due_at"]); due != "" {
				meta += ", " + due
			}
			fmt.Printf("  %-20v p%-2v %-8v %v  (%s)\n", r["id"], r["priority"], r["type"], r["title"], meta)
		}

		fmt.Println("\nRecent activity:")
		acts, err := st.Query(fmt.Sprintf(
			"SELECT kind,summary,occurred_at FROM activities WHERE customer_id=%s ORDER BY occurred_at DESC LIMIT 5",
			sqlStr(cust)))
		if err != nil {
			return err
		}
		if len(acts) == 0 {
			fmt.Println("  (none)")
		}
		for _, r := range acts {
			fmt.Printf("  [%v] %v  (%s ago)\n", r["kind"], r["summary"], ageDays(r["occurred_at"]))
		}

		fmt.Println("\nTrajectory:")
		stages, err := st.Query(fmt.Sprintf(
			"SELECT from_stage,to_stage,reason,occurred_at FROM stage_events WHERE customer_id=%s ORDER BY occurred_at",
			sqlStr(cust)))
		if err != nil {
			return err
		}
		if len(stages) == 0 {
			fmt.Println("  (no recorded transitions)")
		}
		for _, r := range stages {
			reason := ""
			if s := fmt.Sprintf("%v", r["reason"]); s != "" && s != "<nil>" {
				reason = "  (" + s + ")"
			}
			fmt.Printf("  %v -> %v%s\n", r["from_stage"], r["to_stage"], reason)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
