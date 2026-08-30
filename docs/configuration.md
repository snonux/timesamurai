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
| `[general]` | Hostname override and `auto_worktime_login` |
| `[report]` | Report presentation (`color`, `verbose`) |

`store_dir` defaults to `db_dir` when unset in the file. Built-in defaults use
`~/git/worktime` for `db_dir` and `~/git/worktime/timesamuraidb` for `store_dir`.

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
| `TIMESAMURAI_COLOR` | `report.color` |
| `TIMESAMURAI_VERBOSE` | `report.verbose` |

Booleans accept `true` / `false` / `yes` / `no` / `1` / `0` / `on` / `off`
(case-insensitive).

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
