package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	custStage  string
	custHealth string
	custCommit bool
)

var customerCmd = &cobra.Command{
	Use:   "customer",
	Short: "Manage customers",
}

var customerAddCmd = &cobra.Command{
	Use:   "add <id> <name...>",
	Short: "Add a customer",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		id := args[0]
		name := strings.Join(args[1:], " ")
		q := fmt.Sprintf(
			"INSERT INTO customers (id,name,stage,health) VALUES (%s,%s,%s,%s)",
			sqlStr(id), sqlStr(name), sqlStr(custStage), sqlStr(custHealth))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ added customer %s (%s)\n", id, name)
		return maybeCommit(st, custCommit, fmt.Sprintf("customer: add %s", id))
	},
}

func init() {
	customerAddCmd.Flags().StringVar(&custStage, "stage", "onboarding", "initial stage")
	customerAddCmd.Flags().StringVar(&custHealth, "health", "green", "health: green|yellow|red")
	customerAddCmd.Flags().BoolVar(&custCommit, "commit", false, "commit immediately")
	customerCmd.AddCommand(customerAddCmd)
	rootCmd.AddCommand(customerCmd)
}
