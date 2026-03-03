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
			model, err := tuiapp.NewModelWithConfig(currentConfig(cmd))
			if err != nil {
				return err
			}
			program := tea.NewProgram(model)
			return program.Start()
		},
	}
}
