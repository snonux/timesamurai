# timesamurai

![timesamurai](./logo-small.png)

`timesamurai` is a JSONL-backed worktime tracking tool: it tracks work, breaks, and time off
as an append-only JSONL log per host.

Current version: `v0.10.0`. Tracking, crediting/debiting, report/list/search,
modify/delete/undo/edit, migrate/export/import, a legacy flag shim, and shell completions
are all implemented and tested. Ongoing work is now maintenance and small fixes rather than
new subsystems.


## Commands

```
timesamurai work start                    open a work session
timesamurai work stop                     close a work session
timesamurai work status                   show open sessions
timesamurai work add / sub                credit/debit a duration (default tag: work)
timesamurai work day-off                  credit a full day off (8h against "off")
timesamurai work usebuffer                move buffer hours (selfdevelopment) into work
timesamurai work report [range]           print the accounting report
timesamurai work list / search            list/search entries, addressed for modify/delete
timesamurai work modify / delete / undo   modify or delete an entry, or revert the last change
timesamurai work edit                     edit entries in $EDITOR as a text block
timesamurai work migrate                  one-shot import of legacy db.<host>.json files into the JSONL store
timesamurai work export [--strict]        rewrite legacy db.<host>.json from the JSONL store
timesamurai work import                   import legacy report.txt-format lines
timesamurai completion                    generate shell completion scripts
```

Run `timesamurai --help` or `timesamurai work <command> --help` for full flag details.

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
