package cli

import (
	tuiapp "codeberg.org/snonux/timr/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch full-screen TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			model := tuiapp.NewModel()
			program := tea.NewProgram(model)
			return program.Start()
		},
	}
}
