package cmd

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/KazChe/cs/internal/tui"
)

var boardCmd = &cobra.Command{
	Use:   "board",
	Short: "Live parade-style TUI of accounts across journey stages",
	RunE: func(cmd *cobra.Command, args []string) error {
		st := mustStore()
		p := tea.NewProgram(tui.New(st), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(boardCmd)
}
