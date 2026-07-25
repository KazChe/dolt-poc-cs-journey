package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/ui"
)

var (
	itemCust     string
	itemType     string
	itemPriority int
	itemRef      string
	itemDesc     string
	itemCommit   bool
)

var itemCmd = &cobra.Command{
	Use:   "item",
	Short: "Manage customer items (asks, bugs, requests, actions)",
}

var itemAddCmd = &cobra.Command{
	Use:   "add <title...>",
	Short: "Create an item for a customer",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		cust, err := resolveCustomer(st, itemCust)
		if err != nil {
			return err
		}
		title := strings.Join(args, " ")
		id := newID("itm")
		q := fmt.Sprintf(
			"INSERT INTO items (id,customer_id,type,title,description,priority,external_ref) VALUES (%s,%s,%s,%s,%s,%d,%s)",
			sqlStr(id), sqlStr(cust), sqlStr(itemType), sqlStr(title), sqlStr(itemDesc), itemPriority, sqlStr(itemRef))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ %s [%s p%d] %s  (%s)\n", id, itemType, itemPriority, title, cust)
		return maybeCommit(st, itemCommit, fmt.Sprintf("item: %s %s (%s)", id, title, cust))
	},
}

var itemResolveCmd = &cobra.Command{
	Use:   "resolve [id]",
	Short: "Resolve an item (fuzzy-pick from open items if no id given)",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		var id string
		if len(args) > 0 {
			id = args[0]
		} else {
			picked, err := ui.PickItem(st, "status <> 'resolved'")
			if err != nil {
				return err
			}
			id = picked
		}
		q := fmt.Sprintf("UPDATE items SET status='resolved', resolved_at=NOW() WHERE id=%s", sqlStr(id))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ resolved %s\n", id)
		return maybeCommit(st, itemCommit, fmt.Sprintf("item: resolve %s", id))
	},
}

var itemLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List open items (optionally for one customer)",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		where := "status <> 'resolved'"
		if itemCust != "" {
			where += " AND customer_id=" + sqlStr(itemCust)
		}
		rows, err := st.Query(
			"SELECT id,customer_id,type,priority,title,created_at FROM items WHERE " + where + " ORDER BY created_at")
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println("(no open items)")
			return nil
		}
		for _, r := range rows {
			fmt.Printf("%-20v %-9v p%-2v %-8v %v  (%s)\n",
				r["id"], r["customer_id"], r["priority"], r["type"], r["title"], ageDays(r["created_at"]))
		}
		return nil
	},
}

func init() {
	itemAddCmd.Flags().StringVarP(&itemCust, "cust", "c", "", "customer id (omit to fuzzy-pick)")
	itemAddCmd.Flags().StringVarP(&itemType, "type", "t", "action", "item type: bug|feature|question|action")
	itemAddCmd.Flags().IntVarP(&itemPriority, "priority", "p", 2, "priority (1 = highest)")
	itemAddCmd.Flags().StringVar(&itemRef, "ref", "", "external link (jira/zendesk/etc.)")
	itemAddCmd.Flags().StringVarP(&itemDesc, "desc", "d", "", "description")
	itemAddCmd.Flags().BoolVar(&itemCommit, "commit", false, "commit immediately")

	itemResolveCmd.Flags().BoolVar(&itemCommit, "commit", false, "commit immediately")

	itemLsCmd.Flags().StringVarP(&itemCust, "cust", "c", "", "filter by customer id")

	itemCmd.AddCommand(itemAddCmd, itemResolveCmd, itemLsCmd)
	rootCmd.AddCommand(itemCmd)
}
