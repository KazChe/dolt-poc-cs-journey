package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/store"
)

var (
	weekSince string
	weekJSON  bool
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

var weekCmd = &cobra.Command{
	Use:   "week [customer]",
	Short: "Show what changed on items since a point in time (default: 7 days ago)",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()

		baseline, err := resolveSince(st, weekSince)
		if err != nil {
			return err
		}

		where := ""
		if len(args) > 0 {
			c := sqlStr(args[0])
			where = fmt.Sprintf(" WHERE (to_customer_id=%s OR from_customer_id=%s)", c, c)
		}
		q := fmt.Sprintf(
			"SELECT diff_type, COALESCE(to_id,from_id) AS id, COALESCE(to_customer_id,from_customer_id) AS customer_id, "+
				"COALESCE(to_title,from_title) AS title, from_status, to_status "+
				"FROM DOLT_DIFF(%s,'HEAD','items')%s", sqlStr(baseline), where)
		rows, err := st.Query(q)
		if err != nil {
			return err
		}

		if weekJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(rows)
		}

		fmt.Printf("Changes since %s (%d):\n", weekSince, len(rows))
		if len(rows) == 0 {
			fmt.Println("  (nothing changed)")
		}
		for _, r := range rows {
			trans := ""
			if fmt.Sprintf("%v", r["diff_type"]) == "modified" &&
				fmt.Sprintf("%v", r["from_status"]) != fmt.Sprintf("%v", r["to_status"]) {
				trans = fmt.Sprintf("  %v->%v", r["from_status"], r["to_status"])
			}
			fmt.Printf("  [%v] %v %v  %v%s\n", r["diff_type"], r["customer_id"], r["id"], r["title"], trans)
		}
		return nil
	},
}

// resolveSince turns a YYYY-MM-DD into the commit that was HEAD just before that
// date, so DOLT_DIFF(baseline, HEAD) shows everything since. Anything else is
// treated as a literal Dolt revision (HEAD~1, a commit hash, a branch).
func resolveSince(st *store.Store, since string) (string, error) {
	if !dateRe.MatchString(since) {
		return since, nil
	}
	rows, err := st.Query(fmt.Sprintf(
		"SELECT commit_hash FROM dolt_log WHERE date < %s ORDER BY date DESC LIMIT 1", sqlStr(since)))
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		// No commit before that date: fall back to the earliest commit.
		rows, err = st.Query("SELECT commit_hash FROM dolt_log ORDER BY date ASC LIMIT 1")
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "HEAD", nil
		}
	}
	return fmt.Sprintf("%v", rows[0]["commit_hash"]), nil
}

func init() {
	weekCmd.Flags().StringVar(&weekSince, "since", time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
		"a date (YYYY-MM-DD) or a Dolt revision (HEAD~1, a commit hash)")
	weekCmd.Flags().BoolVar(&weekJSON, "json", false, "output rows as JSON (to feed a summary)")
	rootCmd.AddCommand(weekCmd)
}
