package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codeberg.org/snonux/timesamurai/internal/duration"
	"codeberg.org/snonux/timesamurai/internal/timefmt"
	timesamuraiTimer "codeberg.org/snonux/timesamurai/internal/timer"
	"codeberg.org/snonux/timesamurai/internal/worktime"
	"github.com/spf13/cobra"
)

type workContext struct {
	dbDir string
	host  string
}

func newWorkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "work",
		Short: "Work-time tracking operations",
	}

	cmd.AddCommand(newWorkLoginCmd())
	cmd.AddCommand(newWorkLogoutCmd())
	cmd.AddCommand(newWorkAddCmd())
	cmd.AddCommand(newWorkSubCmd())
	cmd.AddCommand(newWorkDayOffCmd())
	cmd.AddCommand(newWorkUseBufferCmd())
	cmd.AddCommand(newWorkReportCmd())
	cmd.AddCommand(newWorkStatusCmd())
	cmd.AddCommand(newWorkEditCmd())
	cmd.AddCommand(newWorkImportCmd())

	return cmd
}

func newWorkLoginCmd() *cobra.Command {
	var category string
	var at string
	var descr string
	var startTimer bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Start work-time tracking",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			when, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entry, err := worktime.Login(ctx.dbDir, ctx.host, category, when, descr)
			if err != nil {
				return err
			}

			message := fmt.Sprintf("Logged in: %s at %s", entry.What, entry.Human)
			if startTimer {
				timerMessage, err := startTimerFromWorkCommand()
				if err != nil {
					return err
				}
				message += "\n" + timerMessage
			}

			return printOutput(cmd, message)
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "work", "Category to log in")
	cmd.Flags().StringVar(&at, "at", "", "Timestamp override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	cmd.Flags().BoolVar(&startTimer, "start-timer", false, "Also start the stopwatch timer")
	return cmd
}

func newWorkLogoutCmd() *cobra.Command {
	var category string
	var at string
	var descr string
	var stopTimer bool

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Stop work-time tracking",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			when, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entry, err := worktime.Logout(ctx.dbDir, ctx.host, category, when, descr)
			if err != nil {
				return err
			}

			message := fmt.Sprintf("Logged out: %s at %s", entry.What, entry.Human)
			if stopTimer {
				timerMessage, err := stopTimerFromWorkCommand()
				if err != nil {
					return err
				}
				message += "\n" + timerMessage
			}

			return printOutput(cmd, message)
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "work", "Category to log out")
	cmd.Flags().StringVar(&at, "at", "", "Timestamp override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	cmd.Flags().BoolVar(&stopTimer, "stop-timer", false, "Also stop the stopwatch timer")
	return cmd
}

func newWorkAddCmd() *cobra.Command {
	var category string
	var at string
	var descr string

	cmd := &cobra.Command{
		Use:   "add <duration>",
		Short: "Add manual work-time duration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			parsedDuration, err := duration.Parse(args[0])
			if err != nil {
				return err
			}

			when, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entry, err := worktime.Add(ctx.dbDir, ctx.host, category, parsedDuration, when, descr)
			if err != nil {
				return err
			}

			return printOutput(cmd, fmt.Sprintf("Added: %s (%s)", entry.What, args[0]))
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "work", "Category")
	cmd.Flags().StringVar(&at, "at", "", "Timestamp override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	return cmd
}

func newWorkSubCmd() *cobra.Command {
	var category string
	var at string
	var descr string

	cmd := &cobra.Command{
		Use:   "sub <duration>",
		Short: "Subtract manual work-time duration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			parsedDuration, err := duration.Parse(args[0])
			if err != nil {
				return err
			}

			when, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entry, err := worktime.Sub(ctx.dbDir, ctx.host, category, parsedDuration, when, descr)
			if err != nil {
				return err
			}

			return printOutput(cmd, fmt.Sprintf("Subtracted: %s (%s)", entry.What, args[0]))
		},
	}

	cmd.Flags().StringVarP(&category, "category", "c", "work", "Category")
	cmd.Flags().StringVar(&at, "at", "", "Timestamp override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	return cmd
}

func newWorkDayOffCmd() *cobra.Command {
	var at string
	var descr string

	cmd := &cobra.Command{
		Use:   "day-off",
		Short: "Add an 8-hour day-off entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			day, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entry, err := worktime.AddDayOff(ctx.dbDir, ctx.host, day, descr)
			if err != nil {
				return err
			}

			dayLabel := time.Unix(entry.Epoch, 0).Format("2006-01-02")
			return printOutput(cmd, fmt.Sprintf("Added day off: 8h on %s", dayLabel))
		},
	}

	cmd.Flags().StringVar(&at, "at", "", "Date/time override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	return cmd
}

func newWorkUseBufferCmd() *cobra.Command {
	var at string
	var descr string

	cmd := &cobra.Command{
		Use:   "use-buffer <duration>",
		Short: "Transfer selfdevelopment buffer time into work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			parsedDuration, err := duration.Parse(args[0])
			if err != nil {
				return err
			}

			when, err := parseAtOrNow(at)
			if err != nil {
				return err
			}

			entries, err := worktime.UseBuffer(ctx.dbDir, ctx.host, parsedDuration, when, descr)
			if err != nil {
				return err
			}

			return printOutput(cmd, fmt.Sprintf("Buffer transfer complete (%d entries).", len(entries)))
		},
	}

	cmd.Flags().StringVar(&at, "at", "", "Timestamp override (unix, ISO, today, yesterday)")
	cmd.Flags().StringVarP(&descr, "descr", "d", "", "Description")
	return cmd
}

func newWorkReportCmd() *cobra.Command {
	var verbose bool
	var noColor bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Print weekly work-time report",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := currentConfig(cmd)

			entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return printOutput(cmd, "No entries found.")
			}

			report, err := worktime.BuildReport(entries, cfg)
			if err != nil {
				return err
			}

			rendered := worktime.FormatReport(report, verbose, !noColor)
			_, err = fmt.Fprint(cmd.OutOrStdout(), rendered)
			return err
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show epoch timestamps in day lines")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	return cmd
}

func newWorkStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current login status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := currentConfig(cmd)
			entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
			if err != nil {
				return err
			}

			active := worktime.ActiveCategories(entries)
			if len(active) == 0 {
				return printOutput(cmd, "Not logged in.")
			}

			return printOutput(cmd, "Logged in: "+strings.Join(active, ", "))
		},
	}
}

func newWorkEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open host DB JSON in $EDITOR",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			db, err := worktime.LoadHost(ctx.dbDir, ctx.host)
			if err != nil {
				return err
			}
			if err := worktime.SaveHost(ctx.dbDir, ctx.host, db); err != nil {
				return err
			}

			editor := strings.TrimSpace(os.Getenv("EDITOR"))
			if editor == "" {
				editor = "vi"
			}

			editorParts := strings.Fields(editor)
			editorCmd := exec.Command(editorParts[0], append(editorParts[1:], workDBPath(ctx.dbDir, ctx.host))...)
			editorCmd.Stdin = os.Stdin
			editorCmd.Stdout = os.Stdout
			editorCmd.Stderr = os.Stderr
			return editorCmd.Run()
		},
	}
}

func newWorkImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import report-format text file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext(cmd)
			if err != nil {
				return err
			}

			file, err := os.Open(args[0])
			if err != nil {
				return err
			}

			imported, err := importReportFile(ctx, file)
			closeErr := file.Close()
			if err != nil {
				if closeErr != nil {
					return errors.Join(err, fmt.Errorf("close import file %q: %w", args[0], closeErr))
				}
				return err
			}
			if closeErr != nil {
				return fmt.Errorf("close import file %q: %w", args[0], closeErr)
			}

			return printOutput(cmd, fmt.Sprintf("Imported %d entries.", imported))
		},
	}
}

func resolveWorkContext(cmd *cobra.Command) (workContext, error) {
	cfg := currentConfig(cmd)
	if strings.TrimSpace(cfg.WorktimeDBDir) == "" {
		return workContext{}, errors.New("worktime_db_dir is empty in config")
	}

	host, err := cfg.EffectiveHostname()
	if err != nil {
		return workContext{}, err
	}

	return workContext{
		dbDir: cfg.WorktimeDBDir,
		host:  host,
	}, nil
}

func parseAtOrNow(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now(), nil
	}
	return timefmt.Parse(value)
}

func importReportFile(ctx workContext, report io.Reader) (int, error) {
	return worktime.ImportReport(ctx.dbDir, ctx.host, report)
}

func workDBPath(dbDir, host string) string {
	return filepath.Join(dbDir, "db."+host+".json")
}

func startTimerFromWorkCommand() (string, error) {
	hasElapsed, err := timerHasElapsed()
	if err != nil {
		return "", err
	}
	return timesamuraiTimer.StartTimer(hasElapsed)
}

func stopTimerFromWorkCommand() (string, error) {
	return timesamuraiTimer.StopTimer()
}
