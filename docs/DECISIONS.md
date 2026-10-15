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

## 2026-10-16 — Org config sources and failures

`--org-config` takes a path (relative to the repo root) or an `https://` URL; any other URL scheme is a config error (exit 2). A missing file is a config error; an unreachable server, a non-200 response or a body over 1 MiB is an infrastructure failure (exit 3), because the repo's config is not at fault. Without an injected client, fetching times out after 30 seconds. There is no environment variable for the org config yet; CI passes the flag.

## 2026-10-16 — The org config pin is checked only when an org config is given

`org_config_sha256` in the repo config is compared against the SHA-256 of the org config's raw bytes whenever `--org-config` is used (URL or path). A pin without an org config is not an error, so developers can run `qgate validate` locally without fetching it. Only a repo config may pin, and only an org config may lock keys.

## 2026-10-16 — Locked keys address the resolved config

Entries in `locked` are dotted paths into the resolved config, the same namespace `qgate validate` prints and `--set` uses, and must name a field the resolved-config schema defines. A key under `profile.` locks that field in every profile. The repo config violates a lock when removing the locked fields from it would change the result: the merged value for a top-level key, or that field of any profile, selected or not, for a profile key. Restating the locked value, or inheriting it through `extends`, is therefore allowed, and a violation in a profile that is not selected is still caught. Errors name the repo profile to fix (`profiles.prod.enforcement`). `--set` violates a lock when it changes the resolved value. `qgate validate` lists each locked key with its value and whether the org config or the builtin defaults set it.

## 2026-10-16 — Layers are validated partially, the merged config completely

Each config file is checked against the schema on its own, except for required properties below the root, so a repo config can override one field of an agent or provider the org config defines. Required properties are checked on the merged config. Errors found after merging name the layer that set the offending path: the last layer containing it, matching keyed-list elements by key and appended-list elements by position.

## 2026-10-16 — `validate` fetches an org config URL

Design §4.1 said `validate` uses no network. With an `https://` org config, resolving the config requires fetching it, so `validate` makes exactly that request and no other (no providers, no VCS). §4.1 is updated accordingly.

## 2026-10-16 — Org config URLs are redacted in output

An org config URL may carry credentials in its user info or query string. Everywhere qgate names it (errors, the `validate` header, locked-key listings) it shows the URL without user info, query or fragment, and HTTP client errors are unwrapped so the full URL is never repeated. The `validate` header names both layers: "from .qgate.yaml over org config <url>".

## 2026-10-16 — The HTTP client is chosen at the composition root

`config` depends on a one-method `HTTPDoer` interface. The CLI supplies an `*http.Client` with a 30-second timeout unless the invocation provides its own (tests inject a TLS test server's client).

## 2026-10-16 — Ingestors return raw severities, not final ones

Design §9 step 3 applies an input's `severity_map` before the format's builtin mapping, so the ingestor cannot settle severity itself. `ingest.Finding` carries the analyzer's label (`raw_severity`), the format's builtin mapping for that finding (`default_severity`, e.g. SARIF `security-severity` when present, else its `level`) and the analyzer's own fingerprint. Design §8.2 is updated to match.

## 2026-10-16 — SARIF goldens use fixtures in each analyzer's shape

Semgrep, CodeQL and Trivy are not installed where ticket 05 was built, so the Seam C fixtures are written by hand in each tool's real SARIF shape (Semgrep's matchBasedId fingerprints and CWE-prefixed tags, CodeQL's rules in query-pack `extensions` referenced through `rule.toolComponent.index`, Trivy's ROOTPATH base and severity tags). Replace them with captured output, and regenerate the goldens with `-update`, the first time the analyzers run in CI.

## 2026-10-16 — Git context only when inputs exist

`qgate run` resolves base, head and the diff only when the config has inputs, so a run with nothing to analyze still works outside a git repository. With inputs, running outside a repository, or with no `--base` and no `origin/HEAD` to take a merge base with, is a usage error (exit 2) that names `--base`. Git runs with only `HOME`, `PATH` and the git-config isolation variables from the environment, non-interactive and with a stable locale.

## 2026-10-16 — What the diff counts as changed

A pure rename (100% similarity) is a changed file with no added lines: under `diff` its findings are out of scope, under `files` they are new. A deleted file's findings are never new. A binary file is changed but has no line ranges, so only its file-level findings are new under `diff`. SARIF results without a file location are skipped with a warning in the input's status.

## 2026-10-16 — Inputs run four at a time; a missing file fails only that input

Inputs are read and normalized in parallel, at most four at once, each into its own result slot, then reported in config order. A missing import file, an unsupported format or mode, or a parse error marks that input `failed` with the reason and the run continues; what a failed input does to the outcome and exit code (required inputs, `on_input_error`) arrives with ticket 09. Category mappings arrive with ticket 07; until then a finding's category is the input's `category_default`, else `quality`.

## 2026-10-16 — `go.uber.org/goleak` for goroutine-leak checks

CLAUDE.md requires concurrent code to be tested for goroutines outliving their tests. `goleak` (test-only) does this; `internal/cli` verifies the whole package in `TestMain`, after every test, including parallel ones, has finished.

## 2026-10-16 — Go 1.26 is the minimum version

`golang.org/x/sync` v0.24.0 requires Go 1.26, so `go mod tidy` raised the `go` directive from 1.25.0 to 1.26.0. Design §3.2 asks for the latest stable Go, which 1.26 is; CI installs the version `go.mod` declares.

## 2026-10-16 — SARIF fingerprints, kinds and locations

- When a result carries full `fingerprints`, qgate uses them in preference to `partialFingerprints`: SARIF defines full fingerprints as the stable identity, partial ones as inputs for computing it. Either way the keys are combined in sorted order.
- Only results of kind `fail` (the SARIF default) are findings. `pass`, `notApplicable`, `informational`, `review` and `open` results are skipped and counted in a warning on the input.
- `artifactLocation.uri` is a URI: it is percent-decoded, and a `uriBaseId` is resolved through the run's `originalUriBaseIds` (following chains). A base of the filesystem root (`file:///`, as Trivy writes for some scans) adds nothing and leaves the URI relative. Absolute results are made repo-relative against the repo root, as git and as the caller spell it (they differ through symlinks), and the CI workspace; anything outside them is dropped.

## 2026-10-16 — Which categories are assessed

A successful input assesses its `category_default` (or `quality`, where its findings go without one), every category in its `assesses`, and the category of every finding it produced. A failed input assesses nothing. Design §12.4 names only the declared categories; counting the categories of real findings keeps a category with new findings from being reported as not assessed.

## 2026-10-16 — `score --report` checks the report, then recomputes

`qgate score --report` first checks the report against itself: `resolved_config` must hash to `config_hash`, the `scoring` section must equal `resolved_config.scoring`, and `gate.degraded` must be true if any input failed. It then recomputes scores and the gate through the same function `run` uses and compares both sections with the stored ones byte for byte in canonical JSON (sorted keys, no insignificant whitespace, numbers as written), the same encoding `config_hash` uses, so a report re-encoded by another tool still matches. Any inconsistency or mismatch is exit 3 and says what differs. A report is not signed: these checks catch edits that leave it inconsistent, not a forger who rewrites every section to agree. Run health beyond input statuses (agent failures, budgets, deadlines) is a fact of the original run, so the stored `gate.degraded` is an input to the recomputation. On a match, the exit code mirrors the gate as `run` would (1 for a failed blocking gate), so CI can re-gate from an artifact.

## 2026-10-16 — Gate check names and values

Gate checks are named `overall_min`, `category_min.<category>`, `max_new.<severity>`, `max_new_tool.<severity>` and `require_assessed.<category>`, listed in that order, with categories sorted and severities from critical down. `expected` reads `>= 70`, `<= 0` or `assessed`; `actual` is the score in whole (floored) points, a count, or a boolean, and `null` for a category that was not assessed. Score comparisons use hundredths, so 69.99 fails `>= 70` and shows as 69.

## 2026-10-16 — Category weights

At least one `scoring.category_weights` entry must be positive (exit 2 otherwise). A category with weight 0 is still scored and shown but does not contribute to the overall score; when every assessed category weighs 0 there is no overall score, and the `overall_min` gate check says so in its note rather than implying nothing was assessed. The weighting that forms the overall score belongs to the scoring algorithm version, like the category scores.

## 2026-10-16 — Checkstyle ingestion

A `<error>`'s `source` attribute is its rule ID (`errcheck` for golangci-lint, `eslint.rules.no-console` for ESLint) and also its title, since Checkstyle has no separate title. Errors without a source, such as ESLint parse errors, get rule ID `unnamed` and a warning. The builtin severity mapping is `error`→high, `warning`→medium, `info`→low, `ignore`→info; an input's `severity_map` applies first. A missing or invalid `line` means file-level. As with SARIF, the Seam C fixtures are hand-written in golangci-lint's and ESLint's output shapes because the linters are not installed; replace them with captured output and run `go test ./internal/ingest/checkstyle -update`.

## 2026-10-16 — Category mappings match namespaced rule IDs

A category mapping matches when every criterion it sets matches: `rule_id` against the namespaced rule ID (`golangci:gocyclo`), `path` against the repo-relative path, and `cwe` against any of the finding's CWEs (`"*"` matches any finding that has one). Patterns use doublestar globs. The first matching mapping wins; otherwise the input's `category_default`, else `quality`.

## 2026-10-16 — What tool dedup merges

Design §9 step 9 merges findings from different inputs with identical fingerprints or identical `(path, start_line, CWE set)`. Fingerprints include the namespaced rule ID (step 5), and input names are unique, so fingerprints from different inputs never collide; the fingerprint criterion is implemented but in practice the location criterion does the merging. The location criterion requires a non-empty CWE set: without it, any two findings from different tools on the same line would merge. Merging is transitive across inputs (union-find), never within a single input. The survivor is the highest severity, then the alphabetically first input name, then canonical order; it keeps its own fingerprint, rule ID and text, and the others become its `merged_sources`, sorted by input and rule ID. All findings are then put in one canonical order (path, line, rule, column, input, fingerprint, message), so the report does not depend on the order of inputs.
