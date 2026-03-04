package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	timesamurai "codeberg.org/snonux/timesamurai/internal"
	"codeberg.org/snonux/timesamurai/internal/config"
	"codeberg.org/snonux/timesamurai/internal/worktime"
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
	var checkDBIntegrity bool

	cmd := &cobra.Command{
		Use:          "timesamurai",
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
				_, err := fmt.Fprintln(cmd.OutOrStdout(), timesamurai.Version)
				return err
			}
			if checkDBIntegrity {
				return runDBIntegrityCheck(cmd)
			}

			return cmd.Help()
		},
	}

	cmd.Flags().BoolVar(&showVersion, "version", false, "Print version and exit")
	cmd.Flags().BoolVar(&checkDBIntegrity, "check-db-integrity", false, "Validate worktime database integrity and exit")
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

func runDBIntegrityCheck(cmd *cobra.Command) error {
	cfg := currentConfig(cmd)
	entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
	if err != nil {
		return err
	}

	issues := worktime.CheckEntriesIntegrity(entries, worktime.DefaultMaxSessionSpan)
	openSessions := worktime.OpenSessions(entries)

	lines := make([]string, 0, len(issues)+len(openSessions)+2)
	if len(issues) == 0 {
		lines = append(lines, "Database integrity check passed.")
	} else {
		lines = append(lines, fmt.Sprintf("Database integrity check found %d issue(s):", len(issues)))
		for idx, issue := range issues {
			lines = append(lines, fmt.Sprintf("%d. %s", idx+1, issue.String()))
		}
	}

	if len(openSessions) > 0 {
		lines = append(lines, fmt.Sprintf("Warning: currently logged in (%d open session(s)):", len(openSessions)))
		for _, session := range openSessions {
			lines = append(lines, fmt.Sprintf(
				"- category=%s source=%s since=%s",
				session.Category,
				session.Login.Source,
				time.Unix(session.Login.Epoch, 0).Format("2006-01-02 15:04:05"),
			))
		}
	}

	_, printErr := fmt.Fprintln(cmd.OutOrStdout(), strings.Join(lines, "\n"))
	if printErr != nil {
		return printErr
	}
	if len(issues) > 0 {
		return errors.New("database integrity check failed")
	}
	return nil
}
