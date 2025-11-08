# timr

A simple command-line tool to track time spent on tasks. It has been primarily coded using Google Gemini CLI and Claude Code CLI.

## About

`timr` is a minimalist stopwatch-style timer that runs in your terminal. It helps you track the time you spend on your work by allowing you to start, stop, and check the status of the timer. The timer's state is saved to your system, so you can pause your work and resume tracking later.

It has been vibe coded!

## Installation

To build and install `timr`, you need to have Go installed on your system. You can then build the executable by running the following command in the project's root directory:

```bash
go build ./...
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
*   `timr live`: Shows a live, full-screen timer with keyboard controls (q: quit, s: start/stop, r: reset).

## Fish Shell Integration

`timr` can be integrated with the fish shell to display the current timer status in your prompt.

### Installation

Run the installation script:

```bash
./install-fish-integration.sh
```

Then update your `fish_prompt` or `fish_right_prompt` function to include the `timr_prompt` function as shown in the script output.
