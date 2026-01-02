package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"codeberg.org/snonux/timr/internal/ascii"
	"codeberg.org/snonux/timr/internal/live"
	"codeberg.org/snonux/timr/internal/timer"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	output, err := runCommand(os.Args)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println(output)
}

func runCommand(args []string) (string, error) {
	if len(args) < 2 {
		printUsage()
		return "", fmt.Errorf("no command specified")
	}

	var err error
	var output string

	switch args[1] {
	case "start":
		rawStatus, err := timer.GetRawStatus()
		if err != nil {
			return "", err
		}
		status, err := strconv.ParseFloat(rawStatus, 64)
		if err != nil {
			return "", err
		}
		output, err = timer.StartTimer(status > 0)
	case "cont":
		fallthrough
	case "continue":
		rawStatus, err := timer.GetRawStatus()
		if err != nil {
			return "", err
		}
		status, err := strconv.ParseFloat(rawStatus, 64)
		if err != nil {
			return "", err
		}
		if status > 0 {
			output, err = timer.StartTimer(true)
		} else {
			output = "Timer is at 0, cannot continue."
		}
	case "stop", "pause":
		output, err = timer.StopTimer()
	case "status":
		if len(args) > 2 {
			switch args[2] {
			case "raw":
				output, err = timer.GetRawStatus()
			case "rawm":
				output, err = timer.GetRawMinutesStatus()
			default:
				printUsage()
				return "", fmt.Errorf("unknown status command: %s", args[2])
			}
		} else {
			output, err = timer.GetStatus()
		}
	case "reset":
		output, err = timer.ResetTimer()
	case "prompt":
		output, err = timer.GetPromptStatus()
	case "track":
		if len(args) < 3 {
			printUsage()
			return "", fmt.Errorf("track command requires a description")
		}
		// Join all arguments after "track" as the description
		description := strings.Join(args[2:], " ")
		output, err = timer.TrackTime(description)
	case "live":
		// Parse optional font flag
		var font string
		if len(args) > 2 && (args[2] == "--font" || args[2] == "-f") {
			if len(args) > 3 {
				font = args[3]
			}
		} else {
			// Select a random font if no argument is given
			rand.Seed(time.Now().UnixNano())
			font = ascii.AllFonts[rand.Intn(len(ascii.AllFonts))]
		}
		p := tea.NewProgram(live.NewModel(font))
		if err := p.Start(); err != nil {
			return "", err
		}
		return "", nil
	default:
		printUsage()
		return "", fmt.Errorf("unknown command: %s", args[1])
	}

	if err != nil {
		return "", err
	}
	return output, nil
}

func printUsage() {
	fmt.Println("Usage: timr <start|continue|stop|pause|status|reset|live|prompt|track <description>>")
	fmt.Println("  live [-f|--font <font>]  : Show live timer with optional font (doom, mono12, rebel, ansi, ansiShadow)")
}
