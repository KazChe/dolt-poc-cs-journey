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
	itemStatus   string
	itemAll      bool
	itemResolved bool
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
	Short: "List items (open by default; --all/--resolved/--status widen the view)",
	Long: "List items for one customer (or all). By default only open items are\n" +
		"shown. Use --resolved to see only resolved items, --all for every status,\n" +
		"or --status <value> to filter to an exact status. Resolved items also show\n" +
		"the date they were resolved.",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()

		statusWhere, err := itemStatusFilter()
		if err != nil {
			return err
		}
		where := statusWhere
		if itemCust != "" {
			if where != "" {
				where += " AND "
			}
			where += "customer_id=" + sqlStr(itemCust)
		}
		query := "SELECT id,customer_id,type,priority,title,status,created_at,resolved_at FROM items"
		if where != "" {
			query += " WHERE " + where
		}
		query += " ORDER BY created_at"

		rows, err := st.Query(query)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Println(itemLsEmptyMsg())
			return nil
		}
		for _, r := range rows {
			// Open items show age; resolved items show the resolution date.
			when := "created " + ageDays(r["created_at"]) + " ago"
			if d := fmtDay(r["resolved_at"]); d != "" {
				when = "resolved " + d
			}
			fmt.Printf("%-20v %-9v p%-2v %-8v %-9v %v  (%s)\n",
				r["id"], r["customer_id"], r["priority"], r["type"], r["status"], r["title"], when)
		}
		return nil
	},
}

// itemStatusFilter builds the SQL status predicate for `item ls` from the
// --status/--resolved/--all flags. Precedence: --status (exact) > --resolved >
// --all > default (open only, i.e. anything not resolved). Returns "" when no
// status predicate should be applied (--all).
func itemStatusFilter() (string, error) {
	switch {
	case itemStatus != "":
		if itemAll || itemResolved {
			return "", fmt.Errorf("--status cannot be combined with --all or --resolved")
		}
		return "status=" + sqlStr(itemStatus), nil
	case itemResolved:
		if itemAll {
			return "", fmt.Errorf("--resolved cannot be combined with --all")
		}
		return "status='resolved'", nil
	case itemAll:
		return "", nil
	default:
		return "status <> 'resolved'", nil
	}
}

// itemLsEmptyMsg describes the empty result in terms of the active filter.
func itemLsEmptyMsg() string {
	switch {
	case itemStatus != "":
		return fmt.Sprintf("(no items with status %q)", itemStatus)
	case itemResolved:
		return "(no resolved items)"
	case itemAll:
		return "(no items)"
	default:
		return "(no open items)"
	}
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
	itemLsCmd.Flags().BoolVar(&itemAll, "all", false, "list items of every status, not just open")
	itemLsCmd.Flags().BoolVar(&itemResolved, "resolved", false, "list only resolved items (shows the resolved date)")
	itemLsCmd.Flags().StringVar(&itemStatus, "status", "", "filter to an exact status (e.g. open, resolved)")

	itemCmd.AddCommand(itemAddCmd, itemResolveCmd, itemLsCmd)
	rootCmd.AddCommand(itemCmd)
}
