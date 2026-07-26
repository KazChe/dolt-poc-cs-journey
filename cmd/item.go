package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/ui"
)

var (
	itemCust     string
	itemType     string
	itemPriority int
	itemRef      string
	itemDesc     string
	itemDue      string
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
		due, err := dueLiteral(itemDue)
		if err != nil {
			return err
		}
		title := strings.Join(args, " ")
		id := newID("itm")
		q := fmt.Sprintf(
			"INSERT INTO items (id,customer_id,type,title,description,priority,external_ref,due_at) VALUES (%s,%s,%s,%s,%s,%d,%s,%s)",
			sqlStr(id), sqlStr(cust), sqlStr(itemType), sqlStr(title), sqlStr(itemDesc), itemPriority, sqlStr(itemRef), due)
		if err := st.Exec(q); err != nil {
			return err
		}
		suffix := ""
		if itemDue != "" {
			suffix = "  due " + itemDue
		}
		fmt.Printf("✓ %s [%s p%d] %s  (%s)%s\n", id, itemType, itemPriority, title, cust, suffix)
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

var itemDueCmd = &cobra.Command{
	Use:   "due <id> [date]",
	Short: "Set or clear an item's due date (YYYY-MM-DD; omit the date to clear)",
	Long: "Set an item's target date, e.g. `cs item due itm-abc123 2026-08-15`.\n" +
		"Omit the date (`cs item due itm-abc123`) to clear it.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		id := args[0]
		var date string
		if len(args) == 2 {
			date = args[1]
		}
		due, err := dueLiteral(date)
		if err != nil {
			return err
		}
		if err := st.Exec(fmt.Sprintf("UPDATE items SET due_at=%s WHERE id=%s", due, sqlStr(id))); err != nil {
			return err
		}
		if date == "" {
			fmt.Printf("✓ cleared due date on %s\n", id)
		} else {
			fmt.Printf("✓ %s due %s\n", id, date)
		}
		msg := fmt.Sprintf("item: due %s cleared", id)
		if date != "" {
			msg = fmt.Sprintf("item: due %s %s", id, date)
		}
		return maybeCommit(st, itemCommit, msg)
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
		query := "SELECT id,customer_id,type,priority,title,status,created_at,resolved_at,due_at FROM items"
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
			// Surface an open item's due date (overdue/upcoming). Resolved items
			// omit it: the date no longer represents pending work.
			if due := dueAnnotation(r["due_at"]); due != "" && fmtDay(r["resolved_at"]) == "" {
				when += ", " + due
			}
			fmt.Printf("%-20v %-9v p%-2v %-8v %-9v %v  (%s)\n",
				r["id"], r["customer_id"], r["priority"], r["type"], r["status"], r["title"], when)
		}
		return nil
	},
}

// dueLiteral validates a YYYY-MM-DD date and returns it as a quoted SQL literal,
// or the SQL keyword NULL when date is empty (meaning "no due date" / "clear").
// A malformed date is rejected rather than silently stored.
func dueLiteral(date string) (string, error) {
	if date == "" {
		return "NULL", nil
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", fmt.Errorf("invalid due date %q: use YYYY-MM-DD", date)
	}
	return sqlStr(date), nil
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
	itemAddCmd.Flags().StringVar(&itemDue, "due", "", "target date (YYYY-MM-DD)")
	itemAddCmd.Flags().BoolVar(&itemCommit, "commit", false, "commit immediately")

	itemResolveCmd.Flags().BoolVar(&itemCommit, "commit", false, "commit immediately")

	itemDueCmd.Flags().BoolVar(&itemCommit, "commit", false, "commit immediately")

	itemLsCmd.Flags().StringVarP(&itemCust, "cust", "c", "", "filter by customer id")
	itemLsCmd.Flags().BoolVar(&itemAll, "all", false, "list items of every status, not just open")
	itemLsCmd.Flags().BoolVar(&itemResolved, "resolved", false, "list only resolved items (shows the resolved date)")
	itemLsCmd.Flags().StringVar(&itemStatus, "status", "", "filter to an exact status (e.g. open, resolved)")

	itemCmd.AddCommand(itemAddCmd, itemResolveCmd, itemDueCmd, itemLsCmd)
	rootCmd.AddCommand(itemCmd)
}
