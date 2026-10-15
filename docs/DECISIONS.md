# Decisions log

Small "simplest reasonable choice" decisions made during implementation (design §1). Architectural decisions go in `docs/adr/` instead.

## 2026-10-15 — Module path is `qgate`

The repository has no remote yet, so the Go module path is the bare name `qgate`. Rename to the hosting path (e.g. `github.com/<org>/qgate`) once a remote exists; it is a mechanical import rewrite.

## 2026-10-15 — Default JSON report path

When no output is configured, `qgate run` writes the JSON report to `qgate-report.json` in the working directory, matching the reference config. The JSON report is always written because it is the canonical record.

## 2026-10-15 — Usage errors are classified at the root

Any error cobra returns before a command starts running (unknown command, flag or argument) is a usage error, exit 2. Errors from inside a command are fatal runtime errors (exit 3) unless the command classifies them otherwise.

## 2026-10-15 — Report field `gate.checks`

The report lists gate checks under `gate.checks` (each with a `check` field), not `gate.rules`, to match the glossary's "gate check".

## 2026-10-15 — staticcheck runs via `go run` in CI

CI runs a pinned staticcheck with `go run honnef.co/go/tools/cmd/staticcheck@<version>` instead of adding it as a module dependency.
