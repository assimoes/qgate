# Decisions log

Small "simplest reasonable choice" decisions made during implementation (design §1). Architectural decisions go in `docs/adr/` instead.

## 2026-10-15 — Module path is `qgate`

The repository has no remote yet, so the Go module path is the bare name `qgate`. Rename to the hosting path (e.g. `github.com/<org>/qgate`) once a remote exists; it is a mechanical import rewrite.

## 2026-10-15 — Default JSON report path

When no output is configured, `qgate run` writes the JSON report to `qgate-report.json` in the working directory, matching the reference config. The JSON report is always written because it is the canonical record.

## 2026-10-15 — Usage errors are classified at the root

Every command's `RunE` is wrapped so its errors are marked as coming from a running command. Any unmarked error is one cobra raised while parsing the command line (unknown command, flag or argument) and is a usage error, exit 2. Marked errors are fatal runtime errors (exit 3) unless the command classified them otherwise. cobra's `help` command is replaced so an unknown help topic also exits 2, and the auto-generated `completion` command is disabled.

## 2026-10-15 — Report field `gate.checks`

The report lists gate checks under `gate.checks` (each with a `check` field), not `gate.rules`, to match the glossary's "gate check".

## 2026-10-15 — staticcheck runs via `go run` in CI

CI runs a pinned staticcheck with `go run honnef.co/go/tools/cmd/staticcheck@<version>` instead of adding it as a module dependency.

## 2026-10-15 — Build metadata via linker variables

`version`, `commit` and `date` are package variables written by `-ldflags -X` and never mutated at run time; this is the standard Go pattern and not the global mutable state design §1 forbids. Without stamping, the commit falls back to the embedded VCS revision and the build date reads `unknown` (the VCS commit time is not a build date).

## 2026-10-15 — No reporter registry yet

The `Reporter` interface exists, but `run` wires the JSON and console reporters directly. The registry keyed by output format arrives with `--output` and the SARIF and Markdown reporters (ticket 11), when there is more than one configurable format to select.
