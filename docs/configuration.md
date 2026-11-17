# timesamurai configuration

This page explains where the config lives and how overrides work; the
authoritative list of options and comments lives in the example file.

## Global config file

- Location: `$XDG_CONFIG_HOME/timesamurai/config.toml` (usually
  `~/.config/timesamurai/config.toml`).
- Style: sectioned tables only — see [config.toml.example](../config.toml.example)
  for a complete, commented reference.
- There is **no** per-project config file. Hexai's `.hexaiconfig.toml` layer is
  not carried over; a single-user time tracker does not need it.

## Sections

| Section | Purpose |
|---|---|
| `[storage]` | `store_dir` (JSONL store) and `db_dir` (legacy `db.*.json` / export target) |
| `[accounting]` | Weekly hours target and tag categories (`plus_for`, `minus_for`, `buffer_for`, `weekend_days`) |
| `[general]` | Hostname override, `auto_worktime_login` and `login_marker` |
| `[report]` | Report presentation (`color`, `verbose`) |
| `[colors]` | Per-role terminal color palette overrides (see below) |

`store_dir` defaults to `db_dir` when unset in the file. Built-in defaults use
`~/git/worktime` for `db_dir` and `~/git/worktime/timesamuraidb` for `store_dir`.

## Login marker

`work start` (and its `login` alias / legacy `--login`) creates the file named
by `general.login_marker` (default `~/.wtloggedin`); `work stop` removes it.
Shell wrappers and prompts can test for it to tell whether you are logged in.
Entries recorded for another machine with `--host` leave the marker alone, a
stop the store refuses because nothing is open still removes a stale marker,
the marker does not distinguish categories (starting `pet` also creates it,
any `stop` removes it), and a failure to touch the marker only prints a
warning. Set `login_marker = ""` in `config.toml` to disable it (an empty
environment variable is ignored, so the env override cannot disable it).

## Precedence

Highest first:

1. Command-line flags: `--config` (root, selects the config file), then `--store` / `--db` on `work`
2. `TIMESAMURAI_*` environment variables
3. `config.toml`
4. Built-in defaults

## Environment overrides

Recognised variables (whitespace-only values are ignored):

| Variable | Maps to |
|---|---|
| `TIMESAMURAI_STORE_DIR` | `storage.store_dir` |
| `TIMESAMURAI_DB_DIR` | `storage.db_dir` |
| `TIMESAMURAI_WEEK_WORK_HOURS` | `accounting.week_work_hours` |
| `TIMESAMURAI_PLUS_FOR` | `accounting.plus_for` (comma-separated) |
| `TIMESAMURAI_MINUS_FOR` | `accounting.minus_for` (comma-separated) |
| `TIMESAMURAI_BUFFER_FOR` | `accounting.buffer_for` (comma-separated) |
| `TIMESAMURAI_WEEKEND_DAYS` | `accounting.weekend_days` (comma-separated; `Mon`..`Sun`) |
| `TIMESAMURAI_HOSTNAME` | `general.hostname` |
| `TIMESAMURAI_AUTO_WORKTIME_LOGIN` | `general.auto_worktime_login` |
| `TIMESAMURAI_LOGIN_MARKER` | `general.login_marker` |
| `TIMESAMURAI_COLOR` | `report.color` |
| `TIMESAMURAI_VERBOSE` | `report.verbose` |

Booleans accept `true` / `false` / `yes` / `no` / `1` / `0` / `on` / `off`
(case-insensitive).

## Terminal colors (tasks n9, m83)

All text output is colored: the report (day labels, category keys and
values, week separators, balances), the `list`/`search` table (every column
except the free-text description), mutation confirmations, warnings, the final `Error:` line, and the headings,
command names and flags in `--help`. Machine-readable output (`--format
json`, `--version`, completion scripts) is never colored, and with colors
off every command prints exactly the same text, minus the escape sequences.

The master switch is `report.color` (`TIMESAMURAI_COLOR` env var,
`--no-color` flag to disable per-run; the flag is accepted at every level,
e.g. `timesamurai --no-color --help`). Colors are additionally disabled by
the standard `NO_COLOR` environment variable, and automatically whenever the
output stream is not a terminal; the standard `CLICOLOR_FORCE` variable
forces colors on (e.g. through a pipe) without overriding an explicit
`report.color = false` or `--no-color`. The decision is made separately for
stdout and stderr, so redirecting only one of them to a file keeps that file
free of escape codes while the other stream stays colored.

Two surfaces are styled outside a command's normal run: `--help` and the
final `Error:` line. They read the config strictly read-only (no legacy
`config.json` migration) and fall back to the default palette when the
config cannot be loaded, so a broken `config.toml` still produces a readable
error. The legacy-config notice (printed before the config, and therefore
the color settings, are known) stays plain.

The `[colors]` section remaps which style each semantic role uses:

| Role | Default | Used for |
|---|---|---|
| `header` | `bold` | table header, `--help` section headings |
| `separator` | `faint` | report week separator lines |
| `success` | `green` | confirmation verbs, open session category |
| `warn` | `yellow` | warnings, dry-run and confirmation prompts, the report's `*` day marker |
| `error` | `red` | the `Error:` label, failed hosts in the migrate report |
| `address` | `cyan` | `<host>:<id>` entry addresses, host names |
| `time` | `blue` | timestamps, report day labels |
| `dim` | `faint` | quiet hints, field names in `--verbose` output |
| `positive` | `green` | balances >= 0 |
| `negative` | `red` | balances < 0 |
| `key` | `magenta` | report category names, entry actions, `--help` commands and flags |
| `value` | `bold` | durations and other figures |
| `tag` | `yellow` | entry tags |

Style names: the 8 base colors (`black` .. `white`), their bright variants
(`brightblack` .. `brightwhite`), `default`, and the attributes `bold`,
`faint`, `italic`, `underline` — space-joined combinations like `bold cyan`
are allowed:

```toml
[colors]
negative = "bold red"
address  = "brightcyan"
```

Unknown roles or style names are config errors.

## Legacy `config.json` migration

Pre-rewrite timesamurai used a flat JSON file at the same directory:

`~/.config/timesamurai/config.json`

On load:

- If **only** `config.json` exists, timesamurai writes `config.toml` once from
  it (mapping `worktime_db_dir` → `storage.db_dir`, `weekworkhours` →
  `accounting.week_work_hours`, `plusfor` / `minusfor` / `bufferfor` /
  `weekendays` → the accounting lists, plus `hostname` /
  `auto_worktime_login`). The JSON file is **left in place**.
- If **both** exist, **TOML wins** and a one-line notice says the JSON is
  ignored.
- `storage.store_dir` was not in the old JSON; migration seeds it with the
  built-in default (`~/git/worktime/timesamuraidb`).

## Two homes for accounting categories

`~/git/worktime/config.json` also defines `weekworkhours` / `plusfor` /
`minusfor` / `bufferfor` for `worktime.rb`. Once timesamurai's TOML is
authoritative, the two must agree or the reports diverge. Changing a category
means changing both places until Ruby retires. Do not edit `worktime.rb`'s
config from Go.

## Dotfiles deployment

The live config is expected to ship from `~/git/dotfiles` via a Rex
`home_timesamurai` task (same pattern as `home_hexai`). Redeploy after edits
with `rex home_timesamurai`.
