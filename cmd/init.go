package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/store"
)

//go:embed schema.sql
var schemaSQL string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create the cs tables in the Dolt repo and commit",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		if err := st.ExecScript(schemaSQL); err != nil {
			return err
		}
		// Migrate repos created before a column existed. The schema uses CREATE
		// TABLE IF NOT EXISTS, so a new column never reaches an already-created
		// table through the script alone; add it idempotently here.
		if err := ensureColumn(st, "items", "due_at", "DATE NULL"); err != nil {
			return err
		}
		if err := st.Commit("cs: init schema"); err != nil {
			return err
		}
		fmt.Println("✓ schema created and committed")
		return nil
	},
}

// ensureColumn adds a column to a table only if it is not already present.
// Dolt has no ALTER TABLE ... ADD COLUMN IF NOT EXISTS, so we probe
// information_schema first, making `cs init` a safe idempotent migration.
func ensureColumn(st *store.Store, table, column, definition string) error {
	rows, err := st.Query(fmt.Sprintf(
		"SELECT COUNT(*) AS n FROM information_schema.columns "+
			"WHERE table_name=%s AND column_name=%s",
		sqlStr(table), sqlStr(column)))
	if err != nil {
		return err
	}
	if len(rows) > 0 && asInt(rows[0]["n"]) > 0 {
		return nil
	}
	return st.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
}

func init() {
	rootCmd.AddCommand(initCmd)
}
