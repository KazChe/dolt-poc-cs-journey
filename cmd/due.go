package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/store"
)

var (
	dueBefore string
	dueQuiet  bool
)

var dueCmd = &cobra.Command{
	Use:   "due",
	Short: "List open items that are overdue or coming due soon",
	Long: "Shows open items with a due date, split into overdue and upcoming.\n" +
		"Overdue items always appear. \"Soon\" defaults to the next 7 days and is\n" +
		"tunable with --before <YYYY-MM-DD>.\n\n" +
		"With --quiet, prints nothing when nothing is due and a single compact line\n" +
		"otherwise, suitable for a shell startup snippet.",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()

		before, err := dueWindow(dueBefore)
		if err != nil {
			return err
		}
		rows, err := dueItems(st, before)
		if err != nil {
			return err
		}

		overdue, upcoming := splitDue(rows)

		if dueQuiet {
			if line := dueQuietLine(overdue, upcoming); line != "" {
				fmt.Println(line)
			}
			return nil
		}

		if len(rows) == 0 {
			fmt.Println("Nothing due. 🎉")
			return nil
		}
		if len(overdue) > 0 {
			fmt.Printf("Overdue (%d):\n", len(overdue))
			for _, r := range overdue {
				fmt.Println("  " + dueLine(r))
			}
		}
		if len(upcoming) > 0 {
			if len(overdue) > 0 {
				fmt.Println()
			}
			fmt.Printf("Due by %s (%d):\n", before.Format("2006-01-02"), len(upcoming))
			for _, r := range upcoming {
				fmt.Println("  " + dueLine(r))
			}
		}
		return nil
	},
}

// defaultDueWindow is the upper bound for "due soon" when --before is unset:
// 7 days from today, at day granularity.
func defaultDueWindow() time.Time {
	return time.Now().Truncate(24 * time.Hour).AddDate(0, 0, 7)
}

// dueWindow resolves the upper bound for "due soon". Empty means the default
// window; otherwise the value must be a YYYY-MM-DD date. The bound is inclusive
// and compared at day granularity.
func dueWindow(before string) (time.Time, error) {
	if before == "" {
		return defaultDueWindow(), nil
	}
	t, err := time.Parse("2006-01-02", before)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --before date %q: use YYYY-MM-DD", before)
	}
	return t.Truncate(24 * time.Hour), nil
}

// dueDateStr formats a due_at value as YYYY-MM-DD, or "" if null/unparseable.
func dueDateStr(v any) string {
	if d, ok := parseDay(v); ok {
		return d.Format("2006-01-02")
	}
	return ""
}

// dueItems returns open items whose due_at is set and falls on or before the
// window bound, OR is already overdue (overdue is always surfaced regardless of
// the window). Ordered by date so the soonest work comes first.
func dueItems(st *store.Store, before time.Time) ([]map[string]any, error) {
	return st.Query(fmt.Sprintf(
		"SELECT id,customer_id,type,title,due_at FROM items "+
			"WHERE status<>'resolved' AND due_at IS NOT NULL AND due_at<=%s "+
			"ORDER BY due_at",
		sqlStr(before.Format("2006-01-02"))))
}

// splitDue partitions due rows into overdue (before today) and upcoming (today
// or later), preserving the incoming date order within each group.
func splitDue(rows []map[string]any) (overdue, upcoming []map[string]any) {
	today := time.Now().Truncate(24 * time.Hour)
	for _, r := range rows {
		d, ok := parseDay(r["due_at"])
		if !ok {
			continue
		}
		if d.Before(today) {
			overdue = append(overdue, r)
		} else {
			upcoming = append(upcoming, r)
		}
	}
	return overdue, upcoming
}

// dueLine renders one due item for the full listing.
func dueLine(r map[string]any) string {
	date := ""
	if d, ok := parseDay(r["due_at"]); ok {
		date = d.Format("2006-01-02")
	}
	return fmt.Sprintf("%-20v %-9v %-8v %v  (%s)", r["id"], r["customer_id"], r["type"], r["title"], date)
}

// dueQuietLine is the one-line summary for shell integration: empty when
// nothing is due, else a compact count like "⏰ 2 overdue · 1 due soon — cs due".
func dueQuietLine(overdue, upcoming []map[string]any) string {
	var parts []string
	if len(overdue) > 0 {
		parts = append(parts, fmt.Sprintf("%d overdue", len(overdue)))
	}
	if len(upcoming) > 0 {
		parts = append(parts, fmt.Sprintf("%d due soon", len(upcoming)))
	}
	if len(parts) == 0 {
		return ""
	}
	return "⏰ " + strings.Join(parts, " · ") + " — cs due"
}

func init() {
	dueCmd.Flags().StringVar(&dueBefore, "before", "", "upper bound for \"soon\" (YYYY-MM-DD; default 7 days out)")
	dueCmd.Flags().BoolVar(&dueQuiet, "quiet", false, "print one compact line, or nothing when nothing is due")
	rootCmd.AddCommand(dueCmd)
}
