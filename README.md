> **🚧 PRE-ALPHA SOFTWARE:** This project is in a pre-alpha state and is intended for my own personal use only. Use at your own risk.

# timesamurai

`timesamurai` is being rebuilt as a JSONL-backed worktime tracking tool.

Current version: `v0.8.1` (skeleton after the pre-rewrite reset).

See [docs/worktime-rewrite-plan.md](docs/worktime-rewrite-plan.md) for the rewrite plan.

## Build

```bash
go install github.com/magefile/mage@latest
mage build
```

Or:

```bash
go build -o timesamurai ./cmd/timesamurai
```

## Usage

```bash
timesamurai --help
timesamurai --version
```

## Coexistence with worktime.rb

Storage layout, once `work migrate` has run:

| | Path | Owner |
|---|---|---|
| Legacy JSON | `db.<host>.json` in `storage.db_dir` (default `~/git/worktime`) | `worktime.rb`, report-only |
| JSONL store | `db.<host>.jsonl` in `storage.store_dir` (default `~/git/worktime/timesamuraidb`) | `timesamurai work`, source of truth |
| Undo log | `undo.<host>.jsonl` in `storage.store_dir` | `timesamurai work` only |

### Legacy files that hold more than one host

`worktime.rb`'s one-file-per-host layout is a convention, not a rule:
`db.archive.json` carries both `mc-lon-mb8477` (4,404 entries) and
`galaxytabs6` (6). Since `worktime.rb` globs `db.*.json` and merges every
section it finds, export deliberately writes such a host **back into the file
that already owns it** rather than creating a fresh `db.<host>.json`. Creating
one would leave the same entries visible twice, doubling those weeks' hours and
desynchronising login/logout pairing until the report aborts with
`Not logged in`.

Two consequences worth knowing:

* `db.archive.json` keeps its name and both sections after migration.
* Its entries predate `worktime.rb` writing a `source` field, so the first
  export adds one to each. That is a one-time normalisation of an archive that
  never changes again; `human` values are preserved verbatim, so no historical
  timestamp is restated.

### worktime.rb is report-only after migration

`work migrate` (`internal/cli/migrate.go`) is a one-shot, one-way import: legacy
`db.<host>.json` files feed the JSONL store once, and from then on the store is that host's
source of truth. Nothing reads `db.<host>.json` back afterwards.

`worktime.rb` keeps working for *reading* — `--report`, `--edit`, etc. still see real data,
because `work export` (`internal/worktime/export.go`) regenerates `db.<host>.json` from the
JSONL store in the exact legacy shape. But that export is the only thing keeping the JSON
file in sync, and it only runs when `timesamurai work export` is actually invoked — nothing
in the codebase currently triggers it automatically after a Go mutation such as `work add`
or `work login`. So if `worktime.rb` is used to *write* something (`--add`, `--login`, a
hand-edit through `--edit`), that write lands only in `db.<host>.json`. It survives until the
next `work export` run, at which point `ExportHost` rebuilds the file fresh from the store
and that Ruby write is gone.

`ExportHost` does not fail silently here: it diffs the on-disk JSON against the fresh export
it is about to write, and if anything on disk has no counterpart in the store it prints a
loud warning to stderr naming every entry about to be discarded. By default it warns and
proceeds — it never refuses to export and never re-imports the discarded entries into the
store, by design (see the package doc comment on `ExportHost` in
`internal/worktime/export.go`). Recovering a Ruby write that got discarded means re-applying
it through a `timesamurai work` command before the next export, not editing the JSON again.

#### `work export --strict`: opting into fail-closed export

`work export --strict` flips that default for one run: if export would discard any on-disk
entry, it refuses to write `db.<host>.json` at all (leaving the file exactly as it was) and
exits nonzero with an error instead of warning-and-overwriting. Internally this is
`worktime.ExportOptions{Strict: true}` threaded through `ExportHost`/`ExportAll`; the error
wraps `worktime.ErrExportWouldDiscard` so scripts can match it with `errors.Is` instead of
parsing text.

Use `--strict` when you want a hard stop during the dual-tool coexistence window — for
example in a cron job or pre-flight check that should fail loudly rather than quietly drop a
`worktime.rb` write, or while manually reconciling a host you know has been edited outside
`timesamurai work`. Leave it off (the default) for routine exports where warn-and-overwrite
is the expected, unattended behavior; passing `--strict` never changes what counts as a
discard, only what happens once one is detected.

In short: **once a host is migrated, run `timesamurai work` for anything that writes.**
`worktime.rb` is fine for reading, but treat every Ruby write as temporary until the next
`work export` — or run `work export --strict` if you'd rather be stopped than lose it.

### Fish dispatcher: `WORKTIME_IMPL`

The `worktime` fish function in `~/git/dotfiles/fish/conf.d/worktime.fish` (a real file,
not a symlink — editing it is separate, out-of-repo work) currently always shells out to
Ruby:

```fish
function worktime
    ruby $WORKTIME_DIR/worktime.rb $argv
end
```

Every other helper (`worktime::login`, `worktime::log`, `worktime::report`, `worktime::add`,
`worktime::status`, ...) and all of the `wt*` abbreviations call `worktime`, never Ruby
directly, so switching what that one function does is enough to switch every entry point.
The intended pattern — **not yet implemented in dotfiles as of this writing** — is an
environment variable, `WORKTIME_IMPL`, that a user can set per-host or globally to pick
`timesamurai work` instead:

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

Default stays `ruby` so nothing changes for an unmigrated host; `WORKTIME_IMPL=go` (or the
`wtgo` abbreviation for a one-off invocation) routes through `timesamurai work` once that
host has been migrated. This is a documentation of the intended shape only — implementing it
is dotfiles-repo work, not part of this task.

### `worktime::supersync_sync` must sync `*.jsonl` too

`worktime::supersync_sync` (`~/git/dotfiles/fish/conf.d/worktime.fish`) is the function that
`git add`s and pushes everything under `$WORKTIME_DIR` for backup/sync. It stages files by
extension:

```fish
find . -name '*.txt' -exec git add {} \;
find . -name '*.json' -exec git add {} \;
find . -name '*.csv' -exec git add {} \;
```

`*.jsonl` was missing from that list, so `db.<host>.jsonl` and `undo.<host>.jsonl` — the
JSONL store — would never get picked up by `git add` and would silently never sync,
regardless of how much history piled up in the store. This has been fixed by adding a
fourth `find` line for `*.jsonl` alongside the other three.

### The `usebuffer` quirk, replicated on purpose

`worktime.rb`'s `usebuffer` method (`~/git/worktime/worktime.rb`, `usebuffer`) does exactly
two hardcoded writes for whatever value it is given:

```ruby
def usebuffer
  value = @opts[:usebuffer].to_i
  @opts[:what] = 'selfdevelopment'
  insert(:add, -value)
  @opts[:what] = 'work'
  insert(:add, value)
end
```

It is not a general transfer between arbitrary categories — it always debits
`selfdevelopment` and credits `work` by the same amount. `internal/worktime/entries.go`'s
`UseBuffer` replicates that exactly: `bufferSourceTag` is hardcoded to `"selfdevelopment"`
and the credit always goes to `WorkTag` ("work"), because the CLI's `work usebuffer
<duration>` takes no tag argument — same as Ruby's `--usebuffer`, there is nothing to
generalize from. The two writes are also not atomic: if the debit succeeds and the credit
then fails, the debit stays on disk (`UseBuffer`'s doc comment spells this out) — matching
Ruby, which has no transaction around its two `insert(:add, ...)` calls either.
