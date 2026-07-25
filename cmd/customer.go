package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var custStage string

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
			"INSERT INTO customers (id,name,stage) VALUES (%s,%s,%s)",
			sqlStr(id), sqlStr(name), sqlStr(custStage))
		if err := st.Exec(q); err != nil {
			return err
		}
		fmt.Printf("✓ added customer %s (%s)\n", id, name)
		return nil
	},
}

func init() {
	customerAddCmd.Flags().StringVar(&custStage, "stage", "onboarding", "initial stage")
	customerCmd.AddCommand(customerAddCmd)
	rootCmd.AddCommand(customerCmd)
}
