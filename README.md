# timr

`timr` is a terminal time-tracking tool that combines:

- a stopwatch timer,
- worktime-style work log tracking,
- weekly reporting,
- and a Bubble Tea TUI.

Current version: `v0.4.0`.

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
go build -o timr ./cmd/timr
```

## CLI Overview

`timr` now uses Cobra and supports global flags:

```bash
timr --version
timr --config /path/to/config.json
```

Top-level command groups:

- `timr timer ...`
- `timr work ...`
- `timr tui`

### `timer` Commands

```bash
timr timer start
timr timer stop
timr timer continue
timr timer reset
timr timer status [--raw|--raw-minutes]
timr timer prompt
timr timer track <description>
timr timer live [-f|--font <font>]
```

### `work` Commands

```bash
timr work login [-c|--category <cat>] [--at <time>] [-d|--descr <text>] [--start-timer]
timr work logout [-c|--category <cat>] [--at <time>] [-d|--descr <text>] [--stop-timer]
timr work add <duration> [-c|--category <cat>] [--at <time>] [-d|--descr <text>]
timr work sub <duration> [-c|--category <cat>] [--at <time>] [-d|--descr <text>]
timr work use-buffer <duration> [--at <time>] [-d|--descr <text>]
timr work status
timr work report [--verbose] [--no-color]
timr work edit
timr work import <file>
```

## TUI

Launch the TUI:

```bash
timr tui
```

The scaffold currently includes:

- tab-based navigation (`Entries`, `Report`, `Timer`),
- vi-style global keys (`Tab`, `gt`, `gT`, `1/2/3`, `?`, `q`, `ZQ`),
- entries browsing/editing flows,
- report browsing,
- timer screen with work login/logout toggle (`l`).

## TUI Screenshots

Screenshots section (`v0.4.0` baseline):

- Entries screen: _to be captured_
- Report screen: _to be captured_
- Timer screen: _to be captured_

## Fish Shell Integration

Install fish prompt helper:

```bash
./install-fish-integration.fish
```

Then add `timr_prompt` to your `fish_prompt` or `fish_right_prompt`.
