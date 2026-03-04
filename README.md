# timesamurai

`timesamurai` is a terminal time-tracking tool that combines:

- a stopwatch timer,
- worktime-style work log tracking,
- weekly reporting,
- and a Bubble Tea TUI.

Current version: `v0.6.0`.

## Installation

Requirements:

- Go (current project targets Go 1.24+)
- [Mage](https://magefile.org/) (optional, recommended)

Build with Mage:

```bash
go install github.com/magefile/mage@latest
mage build
```

Or build directly:

```bash
go build -o timesamurai ./cmd/timesamurai
```

## CLI Overview

`timesamurai` now uses Cobra and supports global flags:

```bash
timesamurai --version
timesamurai --config /path/to/config.json
```

Top-level command groups:

- `timesamurai timer ...`
- `timesamurai work ...`
- `timesamurai tui`

### `timer` Commands

```bash
timesamurai timer start
timesamurai timer stop
timesamurai timer continue
timesamurai timer reset
timesamurai timer status [--raw|--raw-minutes]
timesamurai timer prompt
timesamurai timer track <description>
timesamurai timer live [-f|--font <font>]
```

### `work` Commands

```bash
timesamurai work login [-c|--category <cat>] [--at <time>] [-d|--descr <text>] [--start-timer]
timesamurai work logout [-c|--category <cat>] [--at <time>] [-d|--descr <text>] [--stop-timer]
timesamurai work add <duration> [-c|--category <cat>] [--at <time>] [-d|--descr <text>]
timesamurai work sub <duration> [-c|--category <cat>] [--at <time>] [-d|--descr <text>]
timesamurai work day-off [--at <time>] [-d|--descr <text>]
timesamurai work use-buffer <duration> [--at <time>] [-d|--descr <text>]
timesamurai work status
timesamurai work report [--verbose] [--no-color]
timesamurai work edit
timesamurai work import <file>
```

## TUI

Launch the TUI:

```bash
timesamurai tui
timesamurai tui --disco
```

The TUI includes:

- tab-based navigation (`Entries`, `Report`, `Timer`),
- TaskSamurai-inspired table styling and status bars,
- vi-style global keys (`Tab`, `gt`, `gT`, `1/2/3`, `?`/`H`, `q`, `ZQ`),
- theme controls (`c` randomize theme, `C` reset, `x` toggle disco mode),
- entries timeline table flows (navigate columns with `h/l` and press `Enter` to edit selected field, including date/time/value/description; `D` opens day-off datepicker),
- report browsing,
- timer screen with work login/logout toggle (`l`).

## TUI Screenshots

Screenshots section (`v0.5.1` baseline):

- Entries screen: _to be captured_
- Report screen: _to be captured_
- Timer screen: _to be captured_

## Fish Shell Integration

Install fish prompt helper:

```bash
./install-fish-integration.fish
```

Then add `timesamurai_prompt` to your `fish_prompt` or `fish_right_prompt`.
