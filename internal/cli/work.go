package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/timr/internal/duration"
	"codeberg.org/snonux/timr/internal/timefmt"
	timrTimer "codeberg.org/snonux/timr/internal/timer"
	"codeberg.org/snonux/timr/internal/worktime"
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
			ctx, err := resolveWorkContext()
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
			ctx, err := resolveWorkContext()
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
			ctx, err := resolveWorkContext()
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
			ctx, err := resolveWorkContext()
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

func newWorkUseBufferCmd() *cobra.Command {
	var at string
	var descr string

	cmd := &cobra.Command{
		Use:   "use-buffer <duration>",
		Short: "Transfer selfdevelopment buffer time into work",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := resolveWorkContext()
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
			cfg := CurrentConfig()

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
			cfg := CurrentConfig()
			entries, err := worktime.LoadAll(cfg.WorktimeDBDir)
			if err != nil {
				return err
			}

			active := activeCategories(entries)
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
			ctx, err := resolveWorkContext()
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
			ctx, err := resolveWorkContext()
			if err != nil {
				return err
			}

			imported, err := importReportFile(ctx, args[0])
			if err != nil {
				return err
			}

			return printOutput(cmd, fmt.Sprintf("Imported %d entries.", imported))
		},
	}
}

func resolveWorkContext() (workContext, error) {
	cfg := CurrentConfig()
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

func activeCategories(entries []worktime.Entry) []string {
	status := map[string]bool{}

	for _, entry := range entries {
		category := strings.TrimSpace(entry.What)
		if category == "" {
			category = "work"
		}

		switch strings.ToLower(strings.TrimSpace(entry.Action)) {
		case "login":
			status[category] = true
		case "logout":
			status[category] = false
		}
	}

	active := make([]string, 0)
	for category, isActive := range status {
		if isActive {
			active = append(active, category)
		}
	}
	sort.Strings(active)
	return active
}

func importReportFile(ctx workContext, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	imported := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "lunch:") {
			continue
		}

		when, workHours, lunchHours, offHours, err := parseImportLine(line)
		if err != nil {
			return imported, err
		}

		workSeconds := int64(workHours * float64(time.Hour/time.Second))
		lunchSeconds := int64(lunchHours * float64(time.Hour/time.Second))
		offSeconds := int64(offHours * float64(time.Hour/time.Second))

		if lunchSeconds > 0 {
			workSeconds += lunchSeconds
		}

		if workSeconds > 0 {
			if _, err := worktime.Add(ctx.dbDir, ctx.host, "work", time.Duration(workSeconds)*time.Second, when, ""); err != nil {
				return imported, err
			}
			imported++
		}
		if lunchSeconds > 0 {
			if _, err := worktime.Add(ctx.dbDir, ctx.host, "lunch", time.Duration(lunchSeconds)*time.Second, when, ""); err != nil {
				return imported, err
			}
			imported++
		}
		if offSeconds > 0 {
			if _, err := worktime.Add(ctx.dbDir, ctx.host, "off", time.Duration(offSeconds)*time.Second, when, ""); err != nil {
				return imported, err
			}
			imported++
		}
	}

	if err := scanner.Err(); err != nil {
		return imported, err
	}

	return imported, nil
}

func parseImportLine(line string) (time.Time, float64, float64, float64, error) {
	fields := strings.Fields(line)
	if len(fields) < 7 {
		return time.Time{}, 0, 0, 0, fmt.Errorf("unsupported import line: %q", line)
	}

	dateToken := strings.TrimSuffix(fields[1], ":")
	workToken := fields[2]
	lunchToken := fields[4]
	offToken := fields[6]

	when, err := parseImportDate(dateToken)
	if err != nil {
		return time.Time{}, 0, 0, 0, err
	}

	workHours, err := parseHourToken(workToken)
	if err != nil {
		return time.Time{}, 0, 0, 0, err
	}
	lunchHours, err := parseHourToken(lunchToken)
	if err != nil {
		return time.Time{}, 0, 0, 0, err
	}
	offHours, err := parseHourToken(offToken)
	if err != nil {
		return time.Time{}, 0, 0, 0, err
	}

	return when, workHours, lunchHours, offHours, nil
}

func parseHourToken(token string) (float64, error) {
	clean := strings.TrimSpace(token)
	if idx := strings.Index(clean, ":"); idx >= 0 {
		clean = clean[idx+1:]
	}
	clean = strings.TrimSuffix(clean, "h")

	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hour token %q: %w", token, err)
	}
	return value, nil
}

func parseImportDate(token string) (time.Time, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return time.Time{}, errors.New("import date is empty")
	}

	if parsed, err := timefmt.Parse(trimmed); err == nil {
		return parsed, nil
	}

	layouts := []string{
		"02.01.2006",
		"20060102",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported import date %q", token)
}

func workDBPath(dbDir, host string) string {
	return filepath.Join(dbDir, "db."+host+".json")
}

func startTimerFromWorkCommand() (string, error) {
	rawStatus, err := timrTimer.GetRawStatus()
	if err != nil {
		return "", err
	}
	status, err := strconv.ParseFloat(rawStatus, 64)
	if err != nil {
		return "", err
	}
	return timrTimer.StartTimer(status > 0)
}

func stopTimerFromWorkCommand() (string, error) {
	return timrTimer.StopTimer()
}
