package cli

import (
	tuiapp "codeberg.org/snonux/timesamurai/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	var disco bool

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch full-screen TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			model, err := tuiapp.NewModelWithConfigAndDisco(currentConfig(cmd), disco)
			if err != nil {
				return err
			}
			program := tea.NewProgram(model)
			_, err = program.Run()
			return err
		},
	}

	cmd.Flags().BoolVar(&disco, "disco", false, "Enable disco mode (random theme changes)")
	return cmd
}
