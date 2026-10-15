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

## 2026-10-15 — Builtin profiles and profile defaults

The builtin defaults define `dev`, `staging` and `prod` as empty profiles, so qgate runs at any of the usual stages with no repo config. Every resolved profile starts from fixed defaults (advisory enforcement, `diff` gating scope, no agents, `warn` on input and agent errors, no gate checks), so a profile only states what differs, even one that extends nothing. An unknown stage is a config error that lists the known profiles.

## 2026-10-15 — `--set` paths address the resolved config

`--set` applies to the resolved config `qgate validate` prints, where the selected profile lives under `profile`: `--set profile.gate.overall_min=80`, not `gate.overall_min`. Values are parsed as YAML. The result is validated against the resolved-config schema and semantic checks, and errors are attributed to `--set`.

## 2026-10-15 — Unset interpolation variables are config errors

`${VAR}` in a field marked `x-qgate-interpolate` fails with exit 2 when `VAR` is not set (set-but-empty is allowed). `${...}` in any other field is left as written, so text such as a review rule's description is never rewritten. The schema marks the fields; the interpolator walks the document alongside the schema rather than keeping a separate list.

## 2026-10-15 — Semantic checks cover every profile, as written

Semantic checks run on the layered config before profile resolution, so references in profiles that are not selected are still checked and errors cite the path as written (`profiles.dev.agents[1]`), not the resolved `profile.*`. They run again on the resolved config only when `--set` was used.

## 2026-10-15 — Builtin category weights include dependencies

The builtin `scoring.category_weights` are the reference values plus `dependencies: 10`, since `dependencies` is a builtin category. Category weight maps deep-merge, so a repo config that lists only some categories keeps the builtin weights for the rest.

## 2026-10-15 — Config errors are printed plainly

The final error of a run is printed to stderr as `qgate: <message>` rather than as a structured log record, because it is read by a human in a CI log. Config errors list every problem, one per line, as `<file>: <dotted.path>: <message>`.

## 2026-10-16 — `golang.org/x/text` as a direct dependency

The config validator renders schema errors with `jsonschema`'s `LocalizedString`, which takes a `golang.org/x/text/message` printer. `x/text` is already a dependency of `santhosh-tekuri/jsonschema/v6` (design §3.2), so importing it directly adds nothing to the build.

## 2026-10-16 — `llm.allowed_providers` is checked in ticket 30

Design §5.2 lists "providers outside `llm.allowed_providers`" among the semantic checks. The spec and tickets place it with the egress controls (ticket 30), where it is tested alongside the rest of the data-egress policy, so ticket 02 does not check it.

## 2026-10-16 — `--set` cannot change the stage

`stage` in the resolved config records which profile was selected. `--set stage=…` would change the label without re-selecting the profile, so it is rejected with a pointer to `--stage`.

## 2026-10-16 — Profile schemas list their properties explicitly

`profile` and `resolvedProfile` repeat the profile fields with `additionalProperties: false` instead of sharing them through `$ref` plus `unevaluatedProperties: false`. With the shared form, one invalid field made every sibling field "unevaluated" and produced a false "unknown property" error for each.
