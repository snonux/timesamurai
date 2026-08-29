# `timesamurai`: JSONL-backed worktime tracking, rebuilt from scratch

## Context

`~/git/worktime/worktime.rb` (444 lines) tracks work time as an append-only event log, one
JSON file per host (`db.<hostname>.json`), merged at report time. The Timewarrior
evaluation in that repo established why Timewarrior cannot replace it — no weekly target,
no carry-over balance, and `--usebuffer` needs a negative duration it cannot represent —
and identified the real weaknesses to fix instead: one `what` per entry, and no way to
search, correct or undo a single entry short of `--edit` on raw JSON.

`~/git/timesamurai` already carries a Go port in `internal/worktime` (~1,300 lines plus
~930 lines of tests) reading and writing the same JSON. **This plan replaces that package
with a from-scratch implementation inside timesamurai** and grows the existing
`timesamurai work` command group into the full tool.

Outcome: `timesamurai` becomes the worktime tool. The `worktime` fish function keeps
working unchanged and can flip between Ruby and Go per invocation, so both run side by side
during the transition.

### Why JSONL and not SQLite

The whole dataset — nine years, six hosts — is **12,802 entries / 1.4 MB**. It loads into
memory instantly. SQLite would buy transactions and a query language that a dataset this
size does not need, and would cost a ~300 KB binary blob in git on ~80 sync commits a
month, which git cannot delta-compress because b-tree pages shuffle on insert. Measured:

| | pretty JSON (today) | JSONL |
|---|---|---|
| total on disk | 2,176,826 B | 1,357,667 B (62%) |
| per entry | 170 B across ~8 lines | 106 B on **one line** |
| cost of one new event | full-file rewrite | **append 106 B** |

So the store is `db.<host>.jsonl`: one entry per line, sorted by epoch, one file per host —
which keeps merges conflict-free exactly as today, since each host writes only its own file.

### Scope: reset to a skeleton first

timesamurai is ~10,200 LOC today and most of it is not wanted. Before any rewrite work, the
repo is stripped back:

| | Packages | LOC |
|---|---|---|
| **Keep** | `go.mod`, `go.sum`, `Magefile.go`, `LICENSE`, `cmd/timesamurai/main.go`, `internal/version.go` | ~15 |
| **Delete** | `tui`, `viinput`, `ascii` — the Bubble Tea GUI and its support | 4,922 |
| **Delete** | `timer`, `worktime`, `cli` | 4,321 |
| **Delete** | `config`, `timefmt`, `duration` | 915 |

`go.mod` drops `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2` and
`common-nighthawk/go-figure`; it keeps cobra and mage. `pelletier/go-toml/v2` is added
later with the config package, not during the skeleton reset. `install-fish-integration.fish`
goes too — its `timesamurai_prompt` depends on the removed timer and is deployed nowhere
today. The result must still compile: `mage build` and `go vet ./...` pass against a root
Cobra command that prints help and `--version`.

**Reuse policy.** HEAD is tagged `pre-rewrite` and pushed before anything is deleted, so the
old code stays reachable via `git show pre-rewrite:<path>`. The reset is a clean slate, not
a ban — recover what genuinely fits the new design, and nothing more. The candidates worth
looking at:

- `internal/worktime/db.go` (225) — its `Entry.UnmarshalJSON` tolerates the int/float/string
  `value` encodings present in the real history. Worth keeping verbatim.
- `internal/worktime/report.go` (412) — the weekly report structure, but it must be
  corrected: it aborts on the superseded login and has never been diffed against Ruby.
- `internal/worktime/entries.go` (311) — Login/Logout/Add/Sub/UseBuffer/AddDayOff semantics.
  **Not** its positional `EditEntry(dbDir, hostname, index int)`, which stable `<host>:<id>`
  addressing replaces.
- `internal/worktime/import.go` (153) and `integrity.go` (144) — the `report.txt` parser, and
  detection of the unpaired-login condition.
- `internal/config/config.go` (559) — the accounting defaults, though the format moves to TOML.
- The ~930 lines of `internal/worktime/*_test.go` remain the **behavioural spec**; port them
  from the tag to the new JSONL API rather than writing fresh ones.

### A bug the current implementation has

`timesamurai work report` **cannot render the real database today** — it aborts with
`already logged in for "work" at epoch 1781630083` (`internal/worktime/report.go:149`). The
cause is a dead check in `worktime.rb:163`:

```ruby
error('Already logged in:', e) if login.key?('what')   # literal 'what', never a key
```

Ruby therefore silently discards the superseded login — an `earth` login at epoch
1781618168 (2026-06-16 16:56) never closed before the Mac logged in again — while
timesamurai detects it correctly and then refuses to report at all. The rewrite must **warn
on stderr and keep going**, discarding the superseded login as Ruby effectively does. Do not
fix `worktime.rb`: it is report-only from here, and fixing it would make it start failing on
its own history.

### Decisions taken

| Decision | Choice |
|---|---|
| Home | `~/git/timesamurai`, module `github.com/snonux/timesamurai`; tool named `timesamurai` |
| Approach | Reset to a skeleton first, then rebuild; reuse from the `pre-rewrite` tag only where it fits |
| Dropped | The Bubble Tea TUI and the stopwatch timer — timesamurai becomes the worktime tool |
| Format | **JSONL**, one entry per line, `db.<host>.jsonl`, one file per host |
| Config | **Sectioned TOML**, following `~/git/hexai` — replaces today's flat `config.json` |
| Config deployment | Via `~/git/dotfiles` + a Rexfile `home_timesamurai` task, following `home_hexai` |
| Store location | **Configurable**; deployed as `~/git/worktime/timesamuraidb` |
| Migration | **One-shot.** Legacy JSON → JSONL runs once per host, then never again |
| Coexistence | One-way: export `db.<host>.json`, so `worktime.rb` stays **report-only** |
| CLI | **Cobra**, verbs canonical; `worktime.rb` flags survive as a deletable shim |
| Tag model | Multi-tag, **at most one accounting tag** enforced per entry |
| Durations | Bare integer = seconds (legacy default); `30m`, `1h`, `1h30m`, `45s` also accepted |
| Extras | Search, modify, delete, undo, Cobra completions |
| Not doing | SQLite; real intervals for `add`; a gaps report |

---

## Structure

Extends the existing layout; Cobra (v1.10.2) and Mage are already in place. JSONL needs only
`encoding/json`; the one new dependency is `github.com/pelletier/go-toml/v2` for config,
the same version hexai uses.

```
cmd/timesamurai/main.go        thin Cobra root (help + --version); grows or moves to internal/cli as commands return
internal/version.go            bump
internal/config/            REWRITTEN as sectioned TOML, hexai's split:
    config_types.go     the Config struct and its sections
    config_defaults.go  defaults
    config_load.go      XDG path resolution, file read, precedence
    config_merge.go     section-wise merge
    config_env.go       TIMESAMURAI_* overrides
    config_validate.go  validation with actionable messages
    config_migrate.go   one-shot config.json -> config.toml
internal/worktime/             REWRITTEN FROM SCRATCH
    model.go       Entry, tags, accounting-tag validation
    store.go       JSONL load / append / rewrite, per host
    entries.go     start/stop/add/sub/usebuffer/modify/delete
    query.go       search and filters
    undo.go        append-only undo log + replay
    report.go      the parity report
    migrate.go     one-shot legacy JSON import
    export.go      legacy JSON export for worktime.rb
    legacy.go      the db.*.json codec (read for migrate, write for export)
internal/timefmt/              durations, times, ranges
internal/cli/
    work.go        extended: the verbs below
    worklegacy.go  worktime.rb flag shim   (delete this file when Ruby retires)
    complete.go    dynamic completion functions
    completion.go  the `completion` command
completions/timesamurai.fish   generated and committed
config.toml.example            shipped, like hexai's
docs/configuration.md          shipped, like hexai's
```

Everything not listed above was deleted by the reset. There is no `timer`, `tui`, `viinput`
or `ascii` package any more.

Skill constraints: constructors first, exported before unexported, no package-level state
(inject `*worktime.Store` and `config.Config`), `context.Context` first on anything doing
file I/O, `%w` wrapping, functions under ~50 lines. Magefile gains `Vet` and `Completions`
targets alongside Build/Run/Test/Lint/Install.

## On-disk format

`db.<host>.jsonl` — one entry per line, sorted by `epoch`, LF-terminated:

```jsonl
{"id":410,"action":"login","epoch":1787917475,"host":"earth","tags":["work"]}
{"id":411,"action":"logout","epoch":1787917547,"host":"earth","tags":["work"]}
{"id":412,"action":"add","epoch":1787951450,"host":"earth","value":7200,"tags":["work","blogpost"],"descr":"Wrote up the observability post"}
```

Field order fixed for stable diffs; `value`, `descr` and `tags` omitted when empty.
`value` is a signed integer, so the 243 `--usebuffer` withdrawals (−829.96h) that
Timewarrior could not represent round-trip without loss.

`undo.<host>.jsonl` is a separate append-only log:

```jsonl
{"ts":1787951460,"op":"update","id":412,"before":{...},"after":{...}}
```

**Writing.** A new entry is a single `O_APPEND` write of one line — atomic at this size on
POSIX, so a racing write cannot interleave or clobber. `modify` and `delete` need a rewrite:
load, change, write to `db.<host>.jsonl.tmp`, `fsync`, `rename` into place. Appends stay the
common path; rewrites are rare.

**Reading.** Load every `db.*.jsonl` into memory (1.4 MB total) and filter in Go. No index,
no cache, nothing to invalidate.

**Stable IDs.** `id` is per host and never reused. The next id is `max(id)` across the
entries file *and* the undo log — since the undo log retains a snapshot of every deleted
entry, that maximum is monotonic without any separate counter file to keep in sync.

## Configuration

Sectioned TOML at `$XDG_CONFIG_HOME/timesamurai/config.toml` (falling back to
`os.UserConfigDir()`), following `~/git/hexai` — `pelletier/go-toml/v2`, a merge step per
section, `TIMESAMURAI_*` environment overrides on top, a shipped `config.toml.example`, and
`docs/configuration.md`. hexai's per-project `.hexaiconfig.toml` layer is **not** carried
over; it has no meaning for a single-user time tracker.

```toml
[storage]
# Where the JSONL store lives. Deployed value; a subdirectory of the worktime
# repo, so it syncs with everything else. Defaults to db_dir when unset.
store_dir = "~/git/worktime/timesamuraidb"

# Where worktime.rb's db.*.json live: the migration source, and the target of
# the one-way export that keeps `worktime.rb --report` working.
db_dir = "~/git/worktime"

[accounting]
week_work_hours = 40
plus_for   = ["off", "bank", "bufferuse", "sick"]
minus_for  = ["lunch"]
buffer_for = ["tools", "pet", "selfdevelopment", "workrebalance",
              "compensate", "travel", "rebalance"]
weekend_days = ["Sat", "Sun"]

[general]
hostname = ""                  # override the detected hostname
auto_worktime_login = false

[report]
color = true
verbose = false
```

**Precedence**, highest first: command-line flag (`--store`, `--db`, `--config`) →
`TIMESAMURAI_STORE_DIR` / `TIMESAMURAI_DB_DIR` / … → `config.toml` → built-in defaults. The
flags are also what lets tests and the first migration runs work against a scratch copy.

**Migrating the existing config.** timesamurai reads flat JSON at
`~/.config/timesamurai/config.json` today, with `worktime_db_dir` and the accounting lists.
`config_migrate.go` converts it once into the sectioned TOML and leaves the JSON in place;
if both exist, TOML wins and a one-line notice says the JSON is now ignored.

**Two homes for the accounting categories.** `~/git/worktime/config.json` also defines
`weekworkhours` / `plusfor` / `minusfor` / `bufferfor`, and `worktime.rb` reads it. Once the
TOML is authoritative for timesamurai, the two must agree or the two reports diverge. Seed
`[accounting]` from that file during `work migrate`, and document that changing a category
means changing both places until Ruby retires. Do not edit `worktime.rb`'s config from Go.

**The store directory must actually get committed.** `store_dir` is
`~/git/worktime/timesamuraidb`, inside the existing repo, so no second repo is needed. But
`worktime::supersync_sync` git-adds only `*.txt`, `*.json` and `*.csv`
(`worktime.fish:40-42`) — `.jsonl` files would never be added, and the store would silently
never sync. Adding `find . -name '*.jsonl' -exec git add {} \;` there is required, not
optional. It is out-of-repo work; flag it rather than doing it silently.

If `store_dir` is ever pointed at a different repo, that repo needs its own
commit/pull/push too.

## Dotfiles deployment

The config ships from `~/git/dotfiles` following the `hexai` precedent
(`Rexfile:168-176`), which is the closest existing analogue.

```
~/git/dotfiles/timesamurai/config.toml      new; the file shown above
```

```perl
desc 'Install ~/.config/timesamurai';
task 'home_timesamurai', sub { ensure "$DOT/timesamurai/*" => "$HOME/.config/timesamurai/" };
```

Three points worth being deliberate about:

- **No Linux guard.** `home_hexai` is wrapped in `if ( $^O eq 'linux' )`, but the Mac
  (`MBDVXJ4XKH9C`) is the busiest tracking host, so this task must run everywhere.
- **Picked up automatically.** `task 'home'` (`Rexfile:479-482`) runs every task matching
  `^home_`, so no umbrella task needs editing.
- **The dotfiles repo is public** (`Rexfile:7`). The config holds only paths and category
  names — no secrets — which is why it can live there rather than in
  `~/git/conf_private/dotfiles`.

`ensure` copies rather than symlinks, so redeploying after a config change is
`rex home_timesamurai`.

## Entry addressing

Entries are addressed **`<host>:<id>`** — `earth:412`, `MBDVXJ4XKH9C:2584`. A bare `412`
means the current host. Every `list` / `search` row leads with the full address, so it can be
pasted straight into `modify` or `delete`. This replaces the current package's positional
`EditEntry(dbDir, hostname, index int, ...)`, which breaks on any insert.

## CLI

The existing `timesamurai work` group, filled out. Verbs are canonical.

```
Tracking
  timesamurai work start [tags...]          open an interval     (default tag: work)
  timesamurai work stop  [tags...]          close it
  timesamurai work status                   what is running, plus today and this week

Crediting
  timesamurai work add <duration> [tags...]
  timesamurai work sub <duration> [tags...]
  timesamurai work usebuffer <duration>     move buffer hours into work
  timesamurai work day-off [--at ...]       kept from the current CLI

Reporting
  timesamurai work report [range]           parity report; no range = all history
  timesamurai work list   [range] [filters] entries with their <host>:<id> addresses
  timesamurai work search <text> [filters]  sugar for list matching descr/tags

Editing
  timesamurai work modify <host:id> [--at|--value|--descr|--tags|--action]
  timesamurai work delete <host:id>...      [--dry-run]; confirms when more than one
  timesamurai work undo                     revert the last mutation
  timesamurai work edit [range]             dump to $EDITOR as text, apply the diff back

Maintenance
  timesamurai work migrate [--force]        one-shot; refuses if already migrated
  timesamurai work export                   force the legacy JSON rewrite
  timesamurai work import <file>            the report.txt line parser
  timesamurai completion fish|bash|zsh|powershell
```

`login`/`logout` stay as hidden aliases of `start`/`stop` so existing muscle memory and the
current README keep working.

Flags on `work`: `--at <time>`, `--descr/-d`, `--verbose/-v`, `--db <dir>`, `--store <dir>`.
Root starts with `--help` / `--version` only after the skeleton reset; add `--config` (and
any integrity check) when the config package returns — do not assume pre-rewrite root flags.

Filters shared by `list` and `search`: `--host`, `--tag`, `--action`, `--descr <substr>`,
`--since`, `--until`, `--min`/`--max` (value), `--limit`, `--format table|json`.

`work edit` keeps `worktime.rb --edit`'s spirit but renders the selected entries as an
editable text block, opens `$EDITOR`, parses the result back and applies the differences as
normal mutations — so they land in the undo log instead of being hand-edited into the file.

### Durations, times, ranges

A bare integer keeps its `worktime.rb` meaning — seconds for a duration, epoch for a time —
so the fish functions, which generate exactly that, keep working untouched. Suffixed forms
are the documented way to type it by hand. Parsing lives in `internal/timefmt`.

```
durations   30m   1h   1h30m   45s   2.5h   3600 (bare = seconds, legacy default)
times       --at 09:00   --at yesterday   --at 2026-08-25T09:00   --at -2h
            --epoch 1787951450 (legacy)
ranges      today | yesterday | week | lastweek | month | 2026-08 | <date>..<date>
```

`work report` with no range prints the **entire** history, exactly as `worktime.rb --report`
does — `report.txt` is a full dump and the golden test depends on it.

### Tags on the command line

Bare words after a verb are tags. The first belonging to an accounting category
(`plusfor` / `minusfor` / `bufferfor` from `config.json`, or `work`) becomes the accounting
tag; a second accounting tag is rejected.

```
timesamurai work add 2h work selfdevelopment blogpost   ok (work accounts; rest are labels)
timesamurai work add 8h off bank                        reject: both are plusfor
```

Legacy `what` values outside those lists (`dotfiles`, `bulgarian`, `selfimprovement`) become
non-accounting labels — matching worktime.rb, where they land in the day hash but are never
printed and never reach the balance.

### Legacy shim

`timesamurai work` with any `worktime.rb` flag and no subcommand routes to
`internal/cli/worklegacy.go`:

```
--login  [--what W]                     -> work start W
--logout [--what W]                     -> work stop W
--add N  [--what W] [--epoch E] [-d D]  -> work add N W --at E -d D
--sub N  [--what W]                     -> work sub N W
--usebuffer N                           -> work usebuffer N
--report / --edit / --import F          -> work report / edit / import F
--pomodoro M                            -> the existing timer path
--hours/-H                              -> accepted and ignored (declared but unused in Ruby)
--log                                   -> explicit error naming --login and --logout
```

Short forms map identically. pflag handles `--login` natively; register the single-letter
shorthands (`-l -o -a -s -u -w -r -e -E -d -v -i -p`) alongside.

## Shell completions

Cobra generates bash, zsh, fish and powershell for free. Add `timesamurai completion
<shell>`, commit the fish output at `completions/timesamurai.fish` (matching
`~/git/gt/completions` and `~/git/geheim/completions`), regenerated by `mage completions`.
The old `install-fish-integration.fish` (hand-written `complete -c` block plus
`timesamurai_prompt`) is gone with the timer; do not revive it. Completions are
generate-only into `completions/timesamurai.fish` — no prompt function.

Cobra's fish output drives dynamic completion through the hidden `__complete`, so wire these
up in `internal/cli/complete.go`:

- **tags** — accounting categories from `config.json`, plus distinct tags already in the store
- **`<host>:<id>`** — recent entries, with date and description as the completion
  description, which fish renders in the candidate list
- **ranges** — `today`, `yesterday`, `week`, `lastweek`, `month`
- **hosts** — from the `db.*.jsonl` / `db.*.json` files present

## Report parity

`report.go` must reproduce `ruby worktime.rb --report` byte for byte — 4,663 lines against
the current data. Quirks to replicate deliberately (`worktime.rb:290-383`):

- day key `%a %Y%m%d %V`, value format `" %s:%02.2fh"`
- print order: balance, work, lunch, off, sick, bank, pet, selfdevelopment, buffer
- a zero value prints empty **unless** the key is `work`
- `*` marks weekend days, and any day with `off >= 8h` or `bank >= 8h`
- `minusfor` subtracted once at day level and once at week level, against totals accumulated
  *before* the day-level subtraction
- weekly target = 40h minus that week's `plusfor`; balance accumulates across weeks
- `buffer` counts `bufferfor` **`add` entries only** — login/logout intervals tagged `pet`
  are excluded, because `worktime.rb:159-161` only accumulates in the `when 'add'` branch.
  A worktime.rb inconsistency worth 53.26h; replicate it for parity and note it in the
  README rather than silently fixing it.
- three trailing newlines after each week block
- the superseded-login case: warn, discard the earlier login, keep rendering

One deliberate divergence: `worktime.rb:348-350` raises `NoMethodError` on a day with a
`minusfor` value but no `work` value (`nil - 3600`). Go treats it as zero — a crash is not
output, so this cannot break parity.

## Migration

`migrate.go` reads every `db.*.json` from the legacy dir, preserving `action`, `what`,
`epoch`, `value`, `descr`, `source` exactly, and assigns each entry its stable `id`. Edge
cases in the real data, all reported rather than silently dropped:

- **one unpaired login** — 4,377 logins vs 4,376 logouts (the 2026-06-16 `earth` case)
- **11 `add` entries with `value == 0`** — inert but real; import them
- **243 negative entries** — import cleanly as signed values

`db.archive.json` is not a host file: it holds `mc-lon-mb8477` (4,404) and `galaxytabs6` (6).
Split it by the `host` field into `db.mc-lon-mb8477.jsonl` and `db.galaxytabs6.jsonl` rather
than carrying the filename forward.

`work migrate` runs **once per host**, records the fact in the store, and a second run exits
with "already migrated" changing nothing. `--force` exists only for re-running against a
scratch copy during development.

## Coexistence with worktime.rb

After migration the JSONL store is the source of truth and the flow is **one-way**: every
mutation rewrites `db.<host>.json` in the exact legacy shape (`action`, `what`, `epoch`,
`source`, `human`, `value`, `descr` — struct field order fixed to match, 2-space indent to
match `JSON.pretty_generate`). Nothing is read back from JSON on a migrated host.

Both formats are committed during the transition, so the repo carries ~3.5 MB of data
instead of 2.1 MB; deleting the export the day Ruby retires drops it to 1.4 MB.

State plainly in the README: **once a host is migrated, `worktime.rb` is report-only.**
Reads stay accurate; any Ruby *write* lands in the JSON and is overwritten by the next
export. So "running both" means Go for writing, either for reading. To stop that costing
data silently, the exporter diffs the on-disk JSON against what it last wrote and prints a
loud warning naming the entries about to be discarded — it warns and proceeds; it does not
refuse and does not re-import.

When reading *other* hosts, prefer `db.<host>.jsonl` and fall back to `db.<host>.json`,
which is what lets the fleet migrate one host at a time.

### Shell integration

`~/.config/fish/conf.d/worktime.fish` is a real file in neither repo and not a symlink —
editing it is separate, out-of-repo work. Only the dispatcher at lines 7-9 changes; all
other functions and the 14 abbreviations route through it, and the legacy flags pass
straight to the shim:

```fish
set -q WORKTIME_IMPL; or set -gx WORKTIME_IMPL ruby

function worktime
    switch $WORKTIME_IMPL
        case go
            timesamurai work $argv
        case '*'
            ruby $WORKTIME_DIR/worktime.rb $argv
    end
end

abbr -a wtgo 'WORKTIME_IMPL=go worktime'
```

If `worktime_store_dir` points at a separate repo, `worktime::sync` and
`worktime::supersync` need to cover it too. Ship both snippets in the timesamurai README.

**Pre-existing bug to raise, not guess at:** `worktime::log` (line 113) calls
`worktime --log`, which Ruby rejects as ambiguous between `--login` and `--logout`, so
`wtlog` has been broken. The shim should fail naming both rather than picking one.

## Testing

Table-driven; the skill's 60% coverage target applies.

1. **Config** — section merge, `TIMESAMURAI_*` precedence over TOML, validation messages,
   and the one-shot `config.json` → `config.toml` conversion.
1. **Port the existing spec** — the ~930 lines in `internal/worktime/*_test.go` are ported
   to the new API before the rewrite counts as done, and must pass.
2. **Golden parity test** — migrate the real `db.*.json` into a temp store, render, diff
   against captured `ruby worktime.rb --report` output committed as `testdata/report.golden`
   (4,663 lines). Byte-identical.
3. **Round-trip** — legacy JSON → JSONL → legacy JSON identical modulo key order, across all
   12,802 real events.
4. **Superseded login** — the 2026-06-16 case warns and still renders (the current abort is
   the regression under test).
5. **Append atomicity** — concurrent appends from two goroutines produce no torn or
   interleaved lines.
6. **ID stability** — ids survive modify and delete, are never reused, and `max` is taken
   across both the entries file and the undo log.
7. **Tag validation** — one accounting tag plus labels accepted; two rejected.
8. **Undo** — insert / modify / delete each revert exactly, tags included.
9. **Addressing** — `<host>:<id>` resolves; a bare id defaults to the current host.
10. **Search filters** — each filter and combinations.
11. **Duration parsing** — `3600`, `30m`, `1h`, `1h30m`, `2.5h`, `45s`, and rejections.
12. **One-shot migration** — a second `migrate` is refused; `--force` on a scratch copy
    reproduces an identical store.
13. **Overwrite warning** — a JSON file changed behind the tool is detected and named.

## Verification

```sh
cd ~/git/timesamurai
mage build && mage test && mage vet
gofmt -l . && errcheck ./...          # errcheck is at ~/go/bin/errcheck

# acceptance check, against a scratch COPY of ~/git/worktime
cp -r ~/git/worktime /tmp/wt-scratch
ruby ~/git/worktime/worktime.rb --report > /tmp/ruby.txt
./timesamurai work --db /tmp/wt-scratch --store /tmp/wt-scratch migrate
./timesamurai work --db /tmp/wt-scratch --store /tmp/wt-scratch report > /tmp/go.txt
diff /tmp/ruby.txt /tmp/go.txt        # must be empty (4663 lines)
./timesamurai work --store /tmp/wt-scratch migrate     # must refuse: already migrated

# a separate store repo, and the config that configures it
./timesamurai work --db /tmp/wt-scratch --store /tmp/wt-store migrate
head -3 /tmp/wt-store/db.earth.jsonl

cp config.toml.example /tmp/ts-config.toml   # set storage.store_dir = "/tmp/wt-store"
./timesamurai --config /tmp/ts-config.toml work list --limit 3
env TIMESAMURAI_STORE_DIR=/tmp/wt-scratch \
    ./timesamurai --config /tmp/ts-config.toml work list --limit 3   # env must win

# editing round trip
./timesamurai work --store /tmp/wt-scratch search "Observability" --limit 5
./timesamurai work --store /tmp/wt-scratch modify earth:412 --value 2h
./timesamurai work --store /tmp/wt-scratch undo

# completions
./timesamurai completion fish > completions/timesamurai.fish
fish -c 'source completions/timesamurai.fish; complete -C "timesamurai work mod"'

# the legacy shim, exactly as fish calls it
./timesamurai work --store /tmp/wt-scratch --add 3600 --epoch (date +%s) --what work

# dotfiles deployment
cd ~/git/dotfiles && rex -d home_timesamurai      # dry run first
test -f ~/.config/timesamurai/config.toml
timesamurai work status                           # resolves ~/git/worktime/timesamuraidb
```

The `db.*.json` files in `~/git/worktime` are the only copy of nine years of history. All
migration work happens on `/tmp/wt-scratch` until the round-trip test passes.

## Task list

28 tasks created in `~/git/timesamurai` via `ask`, each tagged, dependency-linked, carrying
the agent-workflow annotation, and referencing this plan. Only `871` is ready; everything
else unblocks behind it.

| Wave | IDs | Tag | What |
|---|---|---|---|
| **Reset** | `871` | reset | Tag `pre-rewrite`, delete all nine internal packages, trim `go.mod`, leave a compiling skeleton |
| Bootstrap | `h61` | plan | Copy this plan to `docs/worktime-rewrite-plan.md` + README pointer |
| Config | `i61` `j61` | config | Sectioned TOML rewrite; `config.json` → TOML migration, example, docs |
| Deploy | `k61` | dotfiles | `dotfiles/timesamurai/config.toml` + Rexfile `home_timesamurai` |
| Parsing | `l61` | timefmt | Durations, times, ranges |
| Store | `m61` `n61` `o61` `p61` `q61` `r61` `s61` `t61` | store | model, JSONL store, legacy codec, migrate, export, undo, entries, query |
| Report | `u61` `v61` | report | Byte-parity report; superseded-login warn-and-continue |
| CLI | `w61` `x61` `y61` `z61` `071` | cli | Tracking/crediting verbs; report+list+search; modify/delete/undo/edit; migrate/export/import; legacy shim |
| Tests | `171` `271` `371` | tests | Port the existing spec; golden parity + round-trip; new store internals |
| Finish | `571` `471` `671` `771` | completions, build, docs, cleanup | Cobra completions; Mage targets; README coexistence rules; delete old code + version bump |

Start with `ask start 871`. Run `ask ready` after each completion to pick up whatever
unblocked. Tasks `871`, `o61`, `p61`, `s61`, `u61`, `i61`, `l61` and `171` each carry an
annotation naming their specific reuse candidate at the `pre-rewrite` tag.

## Out of scope

SQLite, real intervals for `add`, and the gaps report — considered and deselected.
`vacations.txt` and the worktime `README.md` buffer log stay free text. Separately tracked,
not part of this plan: `timewarrior-evaluation.md` in `~/git/worktime` still needs its
corrections applied (the retracted balance-drift claim, the 1,843 → 2,322 overlap count, a
hand-edited output block, and the holiday overclaim).
