package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
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
		if err := st.Commit("cs: init schema"); err != nil {
			return err
		}
		fmt.Println("✓ schema created and committed")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
