package cli

import (
	"fmt"

	timr "codeberg.org/snonux/timr/internal"
	"codeberg.org/snonux/timr/internal/config"
	"github.com/spf13/cobra"
)

var loadedConfig = config.Default()

// Execute runs the root command.
func Execute() error {
	return NewRootCmd().Execute()
}

// NewRootCmd creates the Cobra root command.
func NewRootCmd() *cobra.Command {
	var configPath string
	var showVersion bool

	cmd := &cobra.Command{
		Use:          "timr",
		Short:        "Track time from your terminal",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return nil
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			loadedConfig = cfg
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), timr.Version)
				return err
			}

			return cmd.Help()
		},
	}

	cmd.Flags().BoolVar(&showVersion, "version", false, "Print version and exit")
	cmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to config file")
	cmd.AddCommand(newTimerCmd())

	return cmd
}

// CurrentConfig returns the config loaded in PersistentPreRunE.
func CurrentConfig() config.Config {
	return loadedConfig
}
