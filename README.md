# timr

A simple command-line tool to track time spent on tasks. It has been primarily coded using Google Gemini CLI and Claude Code CLI.

## About

`timr` is a minimalist stopwatch-style timer that runs in your terminal. It helps you track the time you spend on your work by allowing you to start, stop, and check the status of the timer. The timer's state is saved to your system, so you can pause your work and resume tracking later.

It has been vibe coded!

## Installation

To build and install `timr`, you need to have Go installed on your system. The project uses [Mage](https://magefile.org/) for building.

### Using Mage (Recommended)

```bash
# Install mage if you haven't already
go install github.com/magefile/mage@latest

# Build the project
mage build
```

### Direct Go Build

Alternatively, you can build directly with Go:

```bash
go build -o timr ./cmd/timr
```

This will create a `timr` executable in the current directory. To make it accessible from anywhere, you can move it to a directory in your system's `PATH`, such as `/usr/local/bin`:

```bash
sudo mv timr /usr/local/bin/
```

## Usage

`timr` provides the following commands:

*   `timr start`: Starts the timer. If the timer was previously stopped, it will resume from where it left off.
*   `timr stop` or `timr pause`: Stops or pauses the timer. The elapsed time will be saved.
*   `timr cont` or `timr continue`: Continues the stopped timer.
*   `timr status`: Shows the current status of the timer (running or stopped) and the total elapsed time.
*   `timr status raw`: Shows the current elapsed time in seconds, in a raw format.
*   `timr status rawm`: Shows the current elapsed time in minutes, in a raw format.
*   `timr reset`: Resets the timer. This will set the elapsed time to zero.
*   `timr track <description>`: Saves the current elapsed time with a description and resets the timer.
*   `timr live [-f|--font <font>]`: Shows a live, full-screen timer with keyboard controls and optional font selection.
*   `timr prompt`: Shows a compact status for shell prompt integration.

## Live Mode

The live mode provides an interactive, full-screen timer display with the following features:

### Keyboard Controls

*   `s`: Start/Stop the timer
*   `r`: Reset the timer to zero
*   `f`: Cycle through different display fonts
*   `q` or `Ctrl+C`: Quit live mode

### Font Options

When you run `timr live` without specifying a font, it randomly selects from one of the available fonts. You can also specify a font explicitly:

```bash
# Random font (different each time)
timr live

# Specific fonts
timr live -f doom        # Classic figlet style
timr live -f mono12      # Clean monospace blocks
timr live -f rebel       # Stylized shaded blocks
timr live -f ansi        # Simple block numerals
timr live -f ansiShadow  # Box-drawing characters
```

While in live mode, press `f` to cycle through all available fonts in real-time.

### Font Credits

The ASCII art fonts (mono12, rebel, ansi, ansiShadow) are adapted from the [pomo](https://github.com/Bahaaio/pomo) project by Bahaa El Deen Mohamed, licensed under the MIT License.

## Fish Shell Integration

`timr` can be integrated with the fish shell to display the current timer status in your prompt.

### Installation

Run the installation script:

```bash
./install-fish-integration.fish
```

Then update your `fish_prompt` or `fish_right_prompt` function to include the `timr_prompt` function as shown in the script output.
