package cli

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/timr/internal/ascii"
	"codeberg.org/snonux/timr/internal/live"
	timrTimer "codeberg.org/snonux/timr/internal/timer"
	"codeberg.org/snonux/timr/internal/worktime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func newTimerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timer",
		Short: "Stopwatch timer operations",
	}

	cmd.AddCommand(newTimerStartCmd())
	cmd.AddCommand(newTimerStopCmd())
	cmd.AddCommand(newTimerContinueCmd())
	cmd.AddCommand(newTimerResetCmd())
	cmd.AddCommand(newTimerStatusCmd())
	cmd.AddCommand(newTimerPromptCmd())
	cmd.AddCommand(newTimerTrackCmd())
	cmd.AddCommand(newTimerLiveCmd())

	return cmd
}

func newTimerStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the timer",
		RunE: func(cmd *cobra.Command, args []string) error {
			rawStatus, err := timrTimer.GetRawStatus()
			if err != nil {
				return err
			}
			status, err := strconv.ParseFloat(rawStatus, 64)
			if err != nil {
				return err
			}

			output, err := timrTimer.StartTimer(status > 0)
			if err != nil {
				return err
			}
			if err := syncWorktimeWithTimer(true); err != nil {
				return err
			}
			return printOutput(cmd, output)
		},
	}
}

func newTimerStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the timer",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := timrTimer.StopTimer()
			if err != nil {
				return err
			}
			if err := syncWorktimeWithTimer(false); err != nil {
				return err
			}
			return printOutput(cmd, output)
		},
	}
}

func newTimerContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Continue a stopped timer",
		RunE: func(cmd *cobra.Command, args []string) error {
			rawStatus, err := timrTimer.GetRawStatus()
			if err != nil {
				return err
			}
			status, err := strconv.ParseFloat(rawStatus, 64)
			if err != nil {
				return err
			}

			output := "Timer is at 0, cannot continue."
			if status > 0 {
				output, err = timrTimer.StartTimer(true)
				if err != nil {
					return err
				}
			}

			return printOutput(cmd, output)
		},
	}
}

func newTimerResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset the timer",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := timrTimer.ResetTimer()
			if err != nil {
				return err
			}
			return printOutput(cmd, output)
		},
	}
}

func newTimerStatusCmd() *cobra.Command {
	var raw bool
	var rawMinutes bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show timer status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if raw && rawMinutes {
				return errors.New("only one of --raw or --raw-minutes can be set")
			}

			var (
				output string
				err    error
			)

			switch {
			case raw:
				output, err = timrTimer.GetRawStatus()
			case rawMinutes:
				output, err = timrTimer.GetRawMinutesStatus()
			default:
				output, err = timrTimer.GetStatus()
			}
			if err != nil {
				return err
			}

			return printOutput(cmd, output)
		},
	}

	cmd.Flags().BoolVar(&raw, "raw", false, "Show elapsed time in seconds")
	cmd.Flags().BoolVar(&rawMinutes, "raw-minutes", false, "Show elapsed time in minutes")
	return cmd
}

func newTimerPromptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prompt",
		Short: "Show prompt-friendly timer status",
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := timrTimer.GetPromptStatus()
			if err != nil {
				return err
			}
			return printOutput(cmd, output)
		},
	}
}

func newTimerTrackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "track <description>",
		Short: "Track elapsed time to Taskwarrior and reset timer",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := strings.Join(args, " ")
			output, err := timrTimer.TrackTime(description)
			if err != nil {
				return err
			}
			return printOutput(cmd, output)
		},
	}
}

func newTimerLiveCmd() *cobra.Command {
	var font string

	cmd := &cobra.Command{
		Use:   "live",
		Short: "Launch interactive live timer view",
		RunE: func(cmd *cobra.Command, args []string) error {
			selectedFont := strings.TrimSpace(font)
			if selectedFont == "" {
				selectedFont = ascii.AllFonts[rand.IntN(len(ascii.AllFonts))]
			}

			program := tea.NewProgram(live.NewModel(selectedFont))
			return program.Start()
		},
	}

	cmd.Flags().StringVarP(&font, "font", "f", "", "Font for live timer (doom, mono12, rebel, ansi, ansiShadow)")
	return cmd
}

func printOutput(cmd *cobra.Command, output string) error {
	if output == "" {
		return nil
	}

	_, err := fmt.Fprintln(cmd.OutOrStdout(), output)
	return err
}

func syncWorktimeWithTimer(start bool) error {
	cfg := CurrentConfig()
	if !cfg.AutoWorktimeLogin {
		return nil
	}

	ctx, err := resolveWorkContext()
	if err != nil {
		return err
	}

	now := time.Now()
	if start {
		_, err = worktime.Login(ctx.dbDir, ctx.host, "work", now, "auto timer start")
	} else {
		_, err = worktime.Logout(ctx.dbDir, ctx.host, "work", now, "auto timer stop")
	}
	if err == nil {
		return nil
	}

	// Avoid failing timer commands on no-op state sync mismatches.
	if strings.Contains(err.Error(), "already logged in") || strings.Contains(err.Error(), "not logged in") {
		return nil
	}

	return err
}
