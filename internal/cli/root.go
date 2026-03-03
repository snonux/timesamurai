package cli

import (
	"context"
	"fmt"

	timr "codeberg.org/snonux/timr/internal"
	"codeberg.org/snonux/timr/internal/config"
	"github.com/spf13/cobra"
)

type configContextKey struct{}

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

			setConfig(cmd, cfg)
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
	cmd.AddCommand(newWorkCmd())
	cmd.AddCommand(newTUICmd())

	return cmd
}

func setConfig(cmd *cobra.Command, cfg config.Config) {
	baseContext := cmd.Context()
	if baseContext == nil {
		baseContext = context.Background()
	}
	cmd.SetContext(context.WithValue(baseContext, configContextKey{}, cfg))
}

func currentConfig(cmd *cobra.Command) config.Config {
	if cmd == nil {
		return config.Default()
	}

	commandContext := cmd.Context()
	if commandContext == nil {
		return config.Default()
	}

	cfg, ok := commandContext.Value(configContextKey{}).(config.Config)
	if !ok {
		return config.Default()
	}
	return cfg
}
