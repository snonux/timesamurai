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
