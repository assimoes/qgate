# qgate design

The binding technical contract for qgate: invariants, algorithms, formats and delivery order. The *why* and user-facing behavior live in `.scratch/qgate/spec.md`; vocabulary in `GLOSSARY.md`; architectural decisions in `docs/adr/`.

## 1. Rules for implementers

1. Deliver in the phase order of §17. Do not start a phase until the previous phase's acceptance criteria pass in CI. Tickets live in `.scratch/qgate/issues/`; each phase ends with a gate ticket that blocks the next phase.
2. **MUST** is a hard requirement. **SHOULD** is a strong default; any deviation is logged in `docs/DECISIONS.md` with a one-paragraph rationale.
3. Resolve any ambiguity this document leaves open with the simplest reasonable choice and log it in `docs/DECISIONS.md`. Do not block. Decisions that are hard to reverse, surprising and the result of a real trade-off get an ADR in `docs/adr/` instead.
4. No third-party dependencies beyond §3.2 without logging the decision.
5. Keep `go vet ./...`, `staticcheck ./...` and `go test -race ./...` green at the end of every phase.
6. No global mutable state. Pass `context.Context` through every call that does I/O. Log with `log/slog`.

## 2. Invariants

1. **LLMs never produce scores (MUST).** Agents produce findings and annotations. Scores come from a pure, versioned function over new findings, with agent influence bounded by configuration. See ADR-0001.
2. **Analyzers win conflicts (MUST).** When an agent finding overlaps a tool finding, the tool finding is kept and the agent finding is superseded.
3. **Reproducibility (MUST).** `qgate score --report <file>` reproduces the identical scores and gate outcome without calling any analyzer or LLM.
4. **Change-scoped by default (SHOULD).** Default gating scope is `diff`; legacy debt does not block merges.
5. **LLM outages do not block merges by default (SHOULD).** Agent failure yields a degraded run and `warn`, unless `on_agent_error: fail`.
6. **Code under review is untrusted data (MUST).** Repository content is never interpreted as instructions to qgate or its agents (§11.8).
7. **Secrets never leave the process in clear text (MUST).** Keys come from environment variables and never appear in logs, reports or comments.

## 3. Architecture

### 3.1 Pipeline

```
qgate run --stage staging
  1. Config loader      builtin → org config → repo config → profile (extends chain) → --set
  2. Context builder    CI detection, base/head SHAs, diff, changed line ranges
  3. Inputs (parallel)  import | run | exec_plugin → findings, metrics
  4. Normalizer         paths, rule namespacing, severity, category, fingerprint
  5. Excusal            gating scope, baseline, suppressions, tool-to-tool dedup
  6. Agents             reviewers (parallel, per unit of work), then triager
  7. Merge              agent findings vs tool findings and vs each other
  8. Scoring            pure, versioned
  9. Gate               pure → pass | warn | fail
 10. Reporters          console, JSON, SARIF, Markdown, PR summary, PR inline
 11. Exit code          §4.3
```

Steps 3 and 6 are the only nondeterministic ones. Everything from step 7 onward MUST be a pure function of findings, metrics and resolved config.

### 3.2 Stack

| Concern | Choice |
|---|---|
| Language | Go, latest stable; `CGO_ENABLED=0` |
| CLI | `github.com/spf13/cobra` |
| YAML | `gopkg.in/yaml.v3` |
| Config validation | `github.com/santhosh-tekuri/jsonschema/v6` |
| Concurrency | `golang.org/x/sync/errgroup`, `golang.org/x/sync/semaphore` |
| Diff parsing | `github.com/sourcegraph/go-diff`; diffs from shelling out to `git` |
| Globs | `github.com/bmatcuk/doublestar/v4` |
| JSONPath | `github.com/PaesslerAG/jsonpath` |
| Anthropic | `github.com/anthropics/anthropic-sdk-go` |
| OpenAI and compatible | `github.com/openai/openai-go` |
| GitHub / GitLab | `github.com/google/go-github` / `gitlab.com/gitlab-org/api/client-go` |
| HTTP recording (tests) | `gopkg.in/dnaeon/go-vcr.v4` |
| Release | GoReleaser; linux/darwin/windows × amd64/arm64; distroless image |

### 3.3 Modules

`cli`, `config`, `model`, `gitctx`, `ingest` (registry + one package per format), `normalize`, `llm` (interface, adapters, `fake`), `agent`, `prompts` (embedded), `score`, `gate`, `report`, `vcs` (github, gitlab), `cache`. Top-level `schemas/` (config, report, plugin, agent output), `testdata/`, `evals/`.

The CLI exposes one in-process entry point — context, args, env, working directory, stdout, stderr in; exit code out — with the LLM provider factory and HTTP transport injectable. `main` is a thin wrapper. This is the primary test seam (§16).

## 4. CLI

### 4.1 Commands

| Command | Purpose |
|---|---|
| `run` | Full pipeline; the CI entrypoint. |
| `validate` | Print the resolved config for a stage. No network, except fetching an `--org-config` URL. |
| `doctor` | Check git context, input files, provider probes (§10.4), VCS token scopes. |
| `score --report <file>` | Recompute scores and gate from a saved report. No analyzers, no LLM. |
| `baseline create` | Write a baseline of current fingerprints (§9.1). |
| `prompts show <agent>` | Print the assembled system prompt for an agent and stage. |
| `eval --suite <dir>` | Run agents against the labeled eval suite (§16). |
| `version` | Version, commit, build date, scoring algorithm version. |

### 4.2 `run` flags

`--stage` (default `dev`), `--config`, `--org-config`, `--base`, `--head`, `--pr`, `--scope` (`diff|files|repo`), `--no-llm`, `--no-comments`, `--output format=path` (repeatable), `--timeout` (default `10m`), `--cache-dir` (default `.qgate/cache`), `--log-format` (`text|json`), `-v`, `--debug-prompts`, `--set path=value` (repeatable).

Env: `QGATE_CONFIG`, `QGATE_STAGE` (flags win); `QGATE_VCS_TOKEN` (falls back to `GITHUB_TOKEN` on GitHub); provider keys by the name given in `api_key_env`; `NO_COLOR`.

### 4.3 Exit codes (MUST)

| Code | Meaning |
|---|---|
| 0 | Outcome `pass` or `warn`, or any outcome under advisory enforcement. |
| 1 | Outcome `fail` under blocking enforcement. |
| 2 | Configuration or usage error. |
| 3 | Fatal runtime error per the profile's error policy, or failure to write the JSON report. |

Infrastructure failures MUST never exit 1.

## 5. Configuration

### 5.1 Discovery and layering

- Repo config: `--config` / `QGATE_CONFIG`, else `.qgate.yaml`, else `.qgate.yml` at repo root. If none, builtin defaults apply and `run` warns.
- Order, later wins: `builtin → org config → repo config → profiles.<stage> (extends chain) → --set`.
- Org config from `--org-config` (path or HTTPS URL). A URL MAY be pinned with `org_config_sha256` in the repo config; mismatch exits 2.
- The org config MAY declare `locked:` dotted paths. Any later layer changing a locked key is a config error.
- Merge: maps deep-merge; scalars replace; lists replace, except `inputs` (keyed by `name`), `review_rules` (keyed by `id`), `suppressions` (appended) and `agents` (map, deep-merged).
- Profiles: single inheritance via `extends`; cycles are a config error. Abstract profiles (only extended) are allowed; selecting one with `--stage` is permitted.
- `${VAR}` interpolation only in schema-marked fields (`base_url`, `path`, `command` args). API keys are never interpolated; only referenced via `api_key_env`.

### 5.2 Validation

- `schemas/config.schema.json` (JSON Schema 2020-12) is the source of truth. YAML → generic map → schema validation → typed structs.
- Errors MUST report file, dotted path and a readable message.
- Semantic checks: unknown provider, agent or category references; `extends` cycles; locked-key violations; duplicate input names; invalid globs; agent `kind` mismatching its builtin prompt's kind; providers outside `llm.allowed_providers`.

### 5.3 Reference config

```yaml
version: 1

project:
  name: payments-api
  include: ["**/*"]
  exclude: ["vendor/**", "**/testdata/**", "**/*.pb.go"]

scoring:
  version: 1
  severity_weights: { critical: 40, high: 20, medium: 8, low: 2, info: 0 }
  agent_weight_pct: 50
  agent_min_confidence: 70
  agent_max_penalty: 15
  category_weights: { security: 40, quality: 30, maintainability: 20, testing: 10 }

llm:
  allowed_providers: [anthropic, local]
  redact_secrets: true
  exclude_paths: ["**/*.env", "secrets/**", "**/*.pem"]
  budget: { max_input_tokens_per_run: 400000, max_units_per_agent: 60 }

providers:
  anthropic:
    type: anthropic
    api_key_env: QGATE_ANTHROPIC_API_KEY
    max_concurrency: 4
    request_timeout: 90s
  local:
    type: openai_compatible
    base_url: ${QGATE_LOCAL_LLM_URL}
    max_concurrency: 2
    request_timeout: 180s
    capabilities: { context_window: 32768, json_schema: false, tool_calls: false }

inputs:
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif, category_default: security, required: true }
  - name: golangci
    format: checkstyle
    path: reports/golangci.xml
    category_default: quality
    severity_map: { error: high, warning: medium, info: low }
  - { name: go-coverage, format: cobertura, path: reports/coverage.xml }
  - { name: licenses, mode: exec_plugin, command: ["qgate-plugin-licenses"], timeout: 60s }

category_mappings:            # first match wins
  - { match: { cwe: "*" }, category: security }
  - { match: { rule_id: "golangci:gocyclo" }, category: maintainability }

agents:
  security-reviewer:
    kind: reviewer
    provider: anthropic
    model: "<model-id>"
    prompt: { base: builtin://security-reviewer@1, extend: [".qgate/prompts/auth.md"] }
    categories: [security]
    max_findings_per_unit: 10
  quality-reviewer:
    kind: reviewer
    provider: local
    model: "qwen2.5-coder:32b"
    prompt: { base: builtin://quality-reviewer@1 }
    categories: [quality, maintainability]
  triage:
    kind: triager
    provider: anthropic
    model: "<model-id>"
    prompt: { base: builtin://triage@1 }
    applies_to: { min_severity: medium }
    max_findings_per_unit: 10   # input batch size for triagers

review_rules:
  - id: no-raw-sql
    description: >
      All database access goes through sqlc-generated functions.
      No SQL built with string concatenation or fmt.Sprintf.
    severity: high
    category: security
    paths: ["**/*.go"]
    agents: [security-reviewer]

suppressions:
  - { rule: "golangci:lll", paths: ["internal/gen/**"], reason: Generated code, expires: 2027-01-31 }

profiles:
  base:                         # abstract
    enforcement: advisory
    scope: diff
    agents: [security-reviewer, quality-reviewer, triage]
    on_input_error: warn
    on_agent_error: warn
    gate: { max_new: { critical: 0 } }
  dev:
    extends: base
    agents: [quality-reviewer]
  staging:
    extends: base
    enforcement: blocking
    gate:
      overall_min: 70
      category_min: { security: 85 }
      max_new: { critical: 0, high: 0 }
      require_assessed: [security]
      metrics: [{ name: coverage.line, scope: diff, op: ">=", value: 70 }]
  prod:
    extends: staging
    on_input_error: fail
    gate: { category_min: { security: 95 } }

reporting:
  pr: { summary: true, inline: true, inline_min_severity: medium, max_inline_comments: 25 }
  outputs:
    - { format: json, path: qgate-report.json }
    - { format: sarif, path: qgate.sarif }
    - { format: markdown, path: qgate-report.md }
```

## 6. Finding model

This is also the plugin and report wire format, versioned by `schema_version`. JSON uses `snake_case`; severity serializes as a lowercase string.

```go
type Severity int // info, low, medium, high, critical

type Category string // open set; custom categories must appear in scoring.category_weights

type Location struct {
	Path      string // repo-relative, forward slashes
	StartLine int    // 1-based; 0 = file-level
	EndLine   int    // inclusive
	StartCol, EndCol int // optional
}

type Source struct {
	Kind    string // "tool" | "agent"
	Name    string // input or agent name
	Version string // analyzer version, or model id
}

type Status string
const (
	StatusNew        = "new"          // counts toward score and gate
	StatusOutOfScope = "out_of_scope" // outside gating scope
	StatusBaseline   = "baseline"
	StatusSuppressed = "suppressed"
	StatusSuperseded = "superseded"   // folded into a stronger finding
	StatusInvalid    = "invalid"      // failed validation; diagnostics only
)

type Suggestion struct {
	Description          string
	StartLine, EndLine   int
	Replacement          string // full text for [StartLine, EndLine]
}

type Annotation struct { // agent commentary only
	Agent       string
	Verdict     string // triager only: likely_true | likely_false_positive | uncertain
	Explanation string
}

type MergedSource struct { // tool findings folded in during dedup
	Input, RuleID, Fingerprint string
	Severity                   Severity
}

type Finding struct {
	Fingerprint   string
	RuleID        string // see §9 step 2 and §11.5
	Category      Category
	Severity      Severity
	Title, Message string
	Location      Location
	CWE           []string
	Source        Source
	Confidence    *int // 0–100, agent findings only
	Suggestion    *Suggestion
	Status        Status
	StatusNote    string // e.g. suppression reason, invalid reason
	Annotations   []Annotation
	MergedSources []MergedSource
}

type Metric struct {
	Name   string  // coverage.line, coverage.branch, tests.total, tests.failed, tests.skipped
	Value  float64
	Unit   string  // percent, count
	Scope  string  // metric scope: "diff" | "repo" | "file:<path>"
	Source Source
}
```

## 7. Git and CI context

| CI | Head | Base | PR |
|---|---|---|---|
| GitHub Actions | `GITHUB_SHA` or PR head SHA from `GITHUB_EVENT_PATH` | `pull_request.base.sha` | `pull_request.number` |
| GitLab CI | `CI_COMMIT_SHA` | `CI_MERGE_REQUEST_DIFF_BASE_SHA` | `CI_MERGE_REQUEST_IID` |
| Fallback | `HEAD` | `git merge-base HEAD origin/<default>` | none |

Flags override CI detection. A base commit missing from a shallow clone MUST produce an error naming the fix (`fetch-depth: 0` / `GIT_DEPTH: 0`).

Diff: `git diff --no-color --unified=0 --find-renames <base>...<head>` → `ChangedFile{Path, OldPath, Status, AddedRanges}`; only new-side added/modified ranges matter. Apply `project.include`/`exclude`. Binary files have no ranges. A second diff with `--unified=<context_lines>` feeds agent context.

## 8. Inputs

### 8.1 Modes

- `import` (default): read a report file produced earlier.
- `run`: execute `command` (argv, no shell), capture stdout (or read `path`), parse with `format`.
- `exec_plugin`: execute `command`, write one JSON request to stdin, read one JSON response from stdout; stderr goes to debug logs; non-zero exit is an input failure.

All commands run in the repo root with a timeout and a minimal environment plus the input's `env` allowlist.

### 8.2 Formats

| Format | Produces | Notes |
|---|---|---|
| `sarif` | findings | 2.1.0, multiple runs; `level` and `security-severity` → severity; `partialFingerprints` used when present; CWE from rule tags/relationships. |
| `checkstyle` | findings | |
| `cobertura`, `lcov` | metrics | `coverage.line`, `coverage.branch` at `repo` and `diff` metric scope. |
| `junit` | metrics | `tests.total`, `tests.failed`, `tests.skipped`. |
| `json_mapping` | findings | JSONPath `mapping` for items, rule_id, message, path, start_line, severity. |
| plugin response | both | `exec_plugin` only. |

```go
type Ingestor interface {
	Format() string
	Parse(ctx context.Context, r io.Reader, in config.Input, gc *gitctx.Context) (ingest.Result, error)
}
type Result struct { Findings []ingest.Finding; Metrics []model.Metric; Warnings []string }

// ingest.Finding is a model.Finding before normalization, plus what
// normalization needs from the format: the analyzer's raw severity label,
// the format's builtin severity for it, and the analyzer's own fingerprint.
type Finding struct {
	model.Finding
	RawSeverity     string
	DefaultSeverity *model.Severity
	ToolFingerprint string
}
```

Ingestors register by format name; adding one touches only its package and the registry.

### 8.3 Plugin protocol

Request: `schema_version`, `repo_root`, `base_sha`, `head_sha`, `stage`, `changed_files[{path, status, added_ranges}]`, `options`. Response: `schema_version`, `tool{name, version}`, `findings[]` (rule_id, category, severity, title, message, location), `metrics[]`. Both have JSON Schemas; a response failing validation is an input failure. qgate fills `source`, `fingerprint` and `status` and namespaces rule IDs.

### 8.4 Input failures

A missing file, failed command, timeout or parse error marks the input `failed`. If `required: true` or `on_input_error: fail`, the run exits 3 after writing reports. Otherwise the run is degraded and categories that input fed may become not assessed.

## 9. Normalization and excusal

Runs over tool findings, and again (from step 5) over valid agent findings. Each step is a separate function.

1. **Paths.** Repo-relative, forward slashes; strip `file://` and CI workspace prefixes; drop findings in excluded paths.
2. **Rule namespacing.** Tool findings: `<input-name>:<rule>`.
3. **Severity.** Input `severity_map`, then builtin per-format defaults (SARIF `error`→high, `warning`→medium, `note`→low, `none`→info; `security-severity` ≥9.0 critical, ≥7.0 high, ≥4.0 medium, else low). Unmapped → `medium` plus a warning.
4. **Category.** First matching `category_mappings` entry, else input `category_default`, else `quality`.
5. **Fingerprint.** With an analyzer-supplied fingerprint: `sha256(rule_id + tool_fingerprint)`. Otherwise `sha256(rule_id + path + normalized_start_line_content + occurrence_index)`, whitespace collapsed, occurrence index disambiguating identical lines in one file. MUST NOT include line numbers.
6. **Gating scope.** `diff`: `new` if the line range intersects an added range (file-level findings: if the file changed). `files`: `new` if in a changed file. `repo`: all `new`. Otherwise `out_of_scope`.
7. **Baseline.** Fingerprints listed in `baseline.path` → `baseline`.
8. **Suppressions.** Config: rule glob + path glob; expired ones ignored and warned. Inline: `qgate:ignore <rule-id-glob> -- <reason>` on the same line or the line above; reason mandatory, otherwise ignored and reported. Matches → `suppressed`, reason in `status_note`.
9. **Tool-to-tool dedup.** Findings from different inputs with identical fingerprints or identical `(path, start_line, CWE set)` merge. Survivor: highest severity; ties → alphabetically first input name. The survivor keeps its own fingerprint, rule ID and text; the rest go to `merged_sources`.

### 9.1 Baseline files

`qgate baseline create --output .qgate/baseline.json` runs inputs only (no agents) with `scope: repo` and writes sorted fingerprints with rule IDs and paths.

## 10. LLM providers

### 10.1 Interface

Agents depend only on this; never on a vendor SDK.

```go
type Request struct {
	System      string
	Messages    []Message // Role user|assistant, Content
	Schema      *jsonschema.Schema
	SchemaName  string
	MaxTokens   int
	Temperature float64 // default 0
}
type Response struct {
	Text       string
	JSON       json.RawMessage // set when Schema != nil and output validated
	Usage      Usage           // InputTokens, OutputTokens
	Model      string
	StopReason string
}
type Capabilities struct { ContextWindow int; JSONSchema, ToolCalls bool }
type Provider interface {
	Name() string
	Complete(ctx context.Context, req Request) (Response, error)
	Capabilities() Capabilities
}
```

### 10.2 Adapters

`anthropic` (structured output via forced tool call), `openai` (native JSON schema), `azure_openai` (deployment as model), `openai_compatible` (custom `base_url`; capabilities from config, verified by probes), `fake` (tests: canned responses by request hash; simulates invalid JSON, timeouts, 429s, repair success). Bedrock and Vertex are deferred but MUST fit the interface.

### 10.3 Structured output

One wrapper over any provider:

1. Native constrained output if supported.
2. Otherwise append the schema and a "respond with only a JSON object" instruction.
3. Extract JSON (tolerate code fences and surrounding prose), validate.
4. On failure, exactly one repair request with the validation errors and the invalid output.
5. If repair fails, `ErrInvalidOutput`: the unit of work fails, the run continues.

### 10.4 Probes (`doctor`)

Per provider in the active profile: a minimal completion (connectivity, auth), model existence, a trivial structured-output request (native / after repair / failed), and declared vs. discovered context window where metadata exists (Ollama `/api/show`, `/models`).

### 10.5 Reliability and budgets

- `max_concurrency` per provider via weighted semaphore.
- Retry 429, 408, 5xx and network errors: exponential backoff, full jitter, base 1s, max 30s, max 4 attempts, honor `Retry-After`.
- `request_timeout` per call plus the global deadline; with under 10% of the deadline left, pending units are cancelled and the run is degraded.
- No silent fallback. An explicit `fallback: <provider>/<model>` is recorded in the report when used.
- `max_input_tokens_per_run`: estimated before each call (chars ÷ 3.5, or the provider's count endpoint); units that would exceed it are skipped and listed.
- `max_units_per_agent`: reviewer units prioritized by `priority_paths`, then change size descending; triager units by highest severity. The rest are skipped and listed.
- Token usage always reported per agent and provider. Cost only with a configured `pricing` table; no built-in prices.

## 11. Agents

### 11.1 Kinds

| Kind | Unit of work | Output |
|---|---|---|
| `reviewer` | One changed file's hunks (split to fit) plus matching review rules and existing new tool findings for that file | New agent findings, optional suggestions |
| `triager` | A batch of up to `max_findings_per_unit` (default 10) new tool findings at or above `applies_to.min_severity`, all from one file, with surrounding code | Annotations (verdict + explanation), optional suggestion for findings lacking one |

Reviewers run first, in parallel across agents and units; the triager runs after merging (§11.6).

### 11.2 Prompt assembly

Fixed section order: **[1] core preamble** (builtin, immutable: role framing, untrusted-input rules) → **[2] role prompt** (`builtin://<name>@<version>` or a team file replacing it) → **[3] extensions** (`prompt.extend` files, in order) → **[4] review rules** (matching entries, numbered, with IDs) → **[5] output contract** (builtin, immutable). Sections 1 and 5 cannot be removed.

- Builtin prompts are embedded at `prompts/<name>/v<N>.md`, immutable once released.
- Team files are `text/template` with `.Language`, `.Stage`, `.ProjectName`, `.Categories`; a missing variable exits 2.
- `prompt_hash = sha256(assembled system prompt)`, recorded per agent and used in the cache key.

### 11.3 Reviewer context

Per unit: path, language, change status; changed hunks with `context_lines` (default 20) of context, **every line prefixed with its new-side line number** and added lines marked `+`; the full file instead if `include_full_file: true` and it fits in 40% of the context window; existing new tool findings in the file with an instruction not to repeat them; matching review rules. Large files split at hunk boundaries so each unit fits in 60% of the context window. `llm.exclude_paths` never sent; content passes the redactor first when `llm.redact_secrets` is on.

### 11.4 Reviewer output contract

```json
{ "findings": [ {
  "rule_id": "kebab-case slug",
  "review_rule_id": "optional: id of the review rule this enforces",
  "title": "≤120 chars", "message": "string",
  "category": "one of the agent's categories",
  "severity": "info|low|medium|high|critical",
  "path": "must equal the unit's path",
  "start_line": 0, "end_line": 0,
  "confidence": "0-100",
  "suggestion": { "description": "string", "replacement": "full text for start_line..end_line" }
} ] }
```

### 11.5 Validation and rule IDs

Each finding failing a check becomes `invalid` with the reason in `status_note`; drop counts are reported per agent.

- `path` equals the unit's path.
- `start_line ≤ end_line`, within the file, intersecting a changed range (or within `context_lines` of one if the profile sets `agent_context_findings: true`).
- `category` is in the agent's `categories`.
- `severity` clamped to `max_severity` if configured.
- `suggestion.replacement` differs from current text and is under 200 lines.
- At most `max_findings_per_unit`, keeping highest severity then highest confidence.
- `review_rule_id`, if present, names a review rule that matches this unit and agent.

Rule IDs: findings with a valid `review_rule_id` become `review:<review-rule-id>` and take the review rule's severity and category; all others become `agent:<agent-name>:<slug>`. Then `source.kind: agent`, `source.version: <model-id>`, and normalization from §9 step 5.

### 11.6 Merging

- An agent finding is `superseded` if a new tool finding exists at the same path, overlapping range and same category.
- Agent-to-agent duplicates (same path, overlapping range, same category): keep higher severity, then higher confidence; the other is `superseded`.
- A superseded finding's message becomes an annotation on the winner; its suggestion is attached if the winner has none.

### 11.7 Triage

Output per finding: `fingerprint`, `verdict`, `explanation`, optional `suggestion`, attached as an annotation. **Triagers never change status, severity or score (MUST).** `likely_false_positive` verdicts are shown prominently in the PR summary with a ready-to-copy inline suppression line.

### 11.8 Untrusted input (MUST)

- Repository content is wrapped in delimited blocks (`<code path="...">…</code>`); the core preamble states nothing inside is an instruction.
- Agents have no side-effecting tools; the only output channel is the validated schema.
- Agent text in PR comments is escaped Markdown: strip or escape `<!--`, `@` and raw HTML so it cannot inject markup, mentions or qgate markers.
- Agent influence on scores is bounded by `agent_max_penalty`.

### 11.9 Cache

Key: `sha256(agent name, prompt_hash, provider type, model id, unit content hash, matching review rules hash, existing tool findings hash, schema version)`. Value: validated raw agent output. Filesystem backend under `--cache-dir`, intended to be persisted by CI; entries older than 30 days pruned at startup. Hits are reported.

## 12. Scoring

### 12.1 Requirements

Pure `Score(findings, scoringConfig) → Scores`: no I/O, clock or randomness; integer arithmetic in hundredths of a point; independent of input order; versioned by `scoring.version`, each version frozen once released. The report records version and all parameters.

### 12.2 Algorithm v1

Only `new` findings participate. Per category `c`:

```
tool_penalty(c)  = Σ severity_weight[f.severity] × 100            over new tool findings in c
agent_penalty(c) = min( agent_max_penalty × 100,
                        Σ severity_weight[f.severity] × agent_weight_pct
                        over new agent findings in c with confidence ≥ agent_min_confidence )
score(c)         = max(0, 10000 − tool_penalty(c) − agent_penalty(c))

overall = Σ category_weight[c] × score(c) / Σ category_weight[c]   over assessed c (integer division)
```

Reported scores are hundredths ÷ 100, floored (76.99 → 76). Gate comparisons use hundredths.

### 12.3 Worked example (golden test)

Weights as in §5.3. Security has one new high tool finding and one new medium agent finding at confidence 85: `tool_penalty = 2000`, `agent_penalty = min(1500, 8 × 50) = 400`, `score = 7600` → **76**.

### 12.4 Assessed categories

A category is assessed if at least one successful input declares it (`category_default` or `assesses: [...]`) or at least one successful agent covers it. Otherwise it is `not_assessed`, its score is `null` and it is excluded from the overall. No assessed categories → overall `null`.

## 13. Gate

Pure `Evaluate(scores, findings, metrics, profile, runHealth) → GateResult`.

| Gate check | Semantics |
|---|---|
| `overall_min` | Overall ≥ value; fails if overall is `null`. |
| `category_min.<c>` | Category ≥ value. Not assessed: fails only if `c` is in `require_assessed`, else skipped with a note. |
| `max_new.<severity>` | Count of new findings at exactly that severity ≤ value; counts tool findings and agent findings at or above `agent_min_confidence`. |
| `max_new_tool.<severity>` | Same, tool findings only. |
| `require_assessed` | Listed categories must be assessed. |
| `metrics[]` | `{name, scope, op, value}`, `op` in `>= > <= < == !=`; fails if missing unless `optional: true`. |

```
violations = failed gate checks
degraded   = any input or agent failure, budget exhaustion or deadline hit

advisory: outcome = warn if violations or degraded, else pass          (exit 0)
blocking: outcome = fail if violations, else warn if degraded, else pass
          on_input_error: fail / on_agent_error: fail turn those failures into exit 3
```

`GateResult` lists every gate check with expected, actual and pass/fail/skipped.

## 14. Reporting and PR comments

### 14.1 Reporters

- `console` (always): outcome first, then category scores, failed gate checks, top findings; color on TTY unless `NO_COLOR`.
- `json`: canonical, `schemas/report.schema.json`, input to `score --report`.
- `sarif`: new findings only by default, one run per source.
- `markdown`: same content as the PR summary.
- `pr-summary`, `pr-inline`.

A reporter failure never changes the gate outcome; failing to write the JSON report exits 3.

### 14.2 Report

Top-level: `schema_version`, `qgate_version`, `run` (id, timestamps, duration, stage, enforcement, gating scope, base/head SHAs, PR, CI provider and job URL, per-step timings), `config_hash`, `resolved_config` (no secrets; only `api_key_env` names), `scoring` (version, params), `inputs[]` (name, format, status, counts, warnings, error), `agents[]` (name, kind, provider, model, prompt_hash, status, units total/ok/cached/skipped, tokens, findings valid/invalid, fallback used), `findings[]`, `metrics[]`, `scores` (overall, per category, `null` when not assessed), `gate` (outcome, degraded, gate checks), `data_egress[]` (provider → files sent), `warnings[]` (run-level warnings such as expired suppressions; warnings about one input are in its status). `score --report` uses findings, metrics, scoring params and the resolved gate config; its results MUST match the stored `scores` and `gate` exactly.

### 14.3 VCS

```go
type Client interface {
	UpsertSummary(ctx context.Context, pr PRRef, marker, body string) error
	ExistingInlineMarkers(ctx context.Context, pr PRRef) (map[string]bool, error)
	PostInlineComments(ctx context.Context, pr PRRef, comments []InlineComment) error
}
```

GitHub and GitLab REST; platform auto-detected, overridable via `reporting.pr.platform` and `reporting.pr.api_url`.

- **Summary:** one comment per PR and stage, marker `<!-- qgate:summary stage=<stage> -->`, edited on every run. Order: outcome, category scores, failed gate checks (expected vs. actual), new-finding counts by severity, top 15 findings with links, `likely_false_positive` triage notes, degraded notices, agents and models, CI job link. Capped at 60,000 chars with a pointer to artifacts.
- **Inline:** new findings at or above `inline_min_severity`, by severity, capped at `max_inline_comments`. GitHub: one review with event `COMMENT` (never `REQUEST_CHANGES`), ` ```suggestion ` blocks. GitLab: MR discussions with positions, ` ```suggestion:-0+0 ` blocks. Each carries `<!-- qgate:fp=<fingerprint> -->`; existing markers are not re-posted. Only lines in the diff; unpositionable findings go to the summary. Suggestions only when the replacement range lies entirely in the diff.
- **Permissions** (`doctor` prints the minimum): GitHub `pull-requests: write` (plus `security-events: write` for SARIF upload); GitLab project/group token with `api` scope.

## 15. Security and privacy

- Keys and tokens only from env vars, held in memory, redacted from all logs and errors via a redacting `slog.Handler`.
- `llm.redact_secrets` (default on): builtin patterns (cloud keys, private key blocks, JWTs, high-entropy `key = "…"` assignments) plus `llm.redact_patterns`; spans become `[REDACTED:<type>]`.
- `llm.exclude_paths` never sent. `llm.allowed_providers` lockable. `--no-llm` guarantees zero provider calls.
- Report `data_egress` lists files sent per provider.
- Prompts and raw responses are written only with `--debug-prompts`, only under `.qgate/debug/`, never to stdout.

## 16. Testing

Test behavior through three seams only; never assert on internal structure.

- **A — CLI entry point (primary).** Real `git` fixture repos built per test; fixture analyzer outputs; `fake` provider; go-vcr recorded VCS HTTP; provider adapters against a local stub server via the injected transport. Assert on exit code, JSON report, Markdown and recorded requests. Covers everything not listed under B or C, including: config layering and locked keys, validation messages, gating scope, baseline, suppressions, dedup, agent validation and merging, structured-output repair, retries, budgets, cache, degraded runs, comment upsert/dedup/escaping, redaction (no API key in captured logs at any level), `--no-llm`, and `score --report` reproducing `scores` and `gate` byte-for-byte.
- **B — `Score` and `Evaluate`.** Golden test for §12.3. Property tests: order independence; adding a new finding never raises any score; excused findings never change scores; agent penalty ≤ `agent_max_penalty` per category.
- **C — `Ingestor.Parse` per format.** Golden files (regenerate with `-update`) over real outputs from Semgrep, golangci-lint (Checkstyle and SARIF), gosec, Trivy, CodeQL, ESLint, Cobertura, LCOV, JUnit.
- **Evals.** `evals/<case>/` holds a snapshot or patch, expected findings (path, range, category, min severity) and known non-issues. `qgate eval` reports precision, recall, invalid-output rate and tokens per agent. A builtin prompt change requires a new prompt version and a recorded eval run.
- **Performance.** Deterministic-only run with 5,000 findings: under 5 s and 500 MB on a standard CI runner, excluding `run`-mode command time.

## 17. Delivery phases

**Phase 0 — Skeleton.** Module, cobra commands (stubs), config load/merge/profiles/schema validation, `validate`, model, JSON report writer, exit codes, console skeleton, CI for qgate itself.
- `validate --stage staging` prints the resolved §5.3 config; invalid configs error with file, path and message.
- Locked-key violations and `extends` cycles exit 2.
- `run` with no inputs writes a valid report and exits 0.

**Phase 1 — Deterministic gate.** Git context and CI auto-detection (base, head, PR), ingestors `sarif`/`checkstyle`/`cobertura`/`lcov`/`junit`, normalization and excusal (§9) except baseline and inline suppressions, config suppressions, scoring, gate, `score --report`, console/JSON/SARIF/Markdown reporters, input modes `import` and `run`.
- Findings outside changed lines are `out_of_scope` and don't affect scores.
- §12.3 produces exactly 76.
- Blocking with a violation exits 1; advisory exits 0 with `warn`; missing required input with `on_input_error: fail` exits 3.
- `score --report` reproduces `scores` and `gate` byte-for-byte.
- Fingerprints survive lines inserted above a finding.
- Tool-to-tool dedup is independent of input order.

**Phase 2 — PR reporting.** VCS interface, GitHub and GitLab clients, summary upsert, inline comments with suggestions and marker dedup, `doctor` git/input/token checks.
- A second run on the same PR edits the summary and posts no duplicate inline comments.
- Suggestions outside the diff are not posted as suggestion blocks.
- Text containing `<!--`, `@user` and HTML is escaped.

**Phase 3 — LLM providers.** Interface, four adapters, `fake`, structured output with repair, retries, concurrency, deadline, explicit fallback, the redactor and redacting log handler, `doctor` probes.
- Structured output works natively and via prompt + repair.
- Retries honor `Retry-After` and stop after 4 attempts.
- The redactor removes its whole test corpus; no API key appears in any log line at any level.
- `doctor` reports probe results for an OpenAI-compatible stub server.

**Phase 4 — Agents.** Prompt assembly, builtin `security-reviewer@1`, `quality-reviewer@1`, `triage@1`; context assembly and chunking; output validation; review-rule rule IDs; merging; triage; agent failure policy and budgets (§10.5); egress controls (redaction applied, `exclude_paths`, `allowed_providers`, `data_egress`, `--debug-prompts`); cache; `prompts show`; `--no-llm`.
- Wrong paths, out-of-diff lines, unknown categories, unknown review rule IDs and oversized suggestions are all `invalid` with reasons.
- An agent finding overlapping a tool finding is `superseded` with its text as an annotation; agent-vs-agent duplicates supersede the weaker one.
- A finding citing `no-raw-sql` has rule ID `review:no-raw-sql` and the review rule's severity and category.
- Agent penalty respects `agent_max_penalty`; triage never changes any score.
- A second run on the same commit is served fully from cache.
- A provider failure under blocking enforcement with default `on_agent_error: warn` yields `warn`, exit 0 and a degraded notice.

**Phase 5 — Extensibility and hardening.** `json_mapping`, `exec_plugin` and protocol schemas, baseline files, inline suppressions, eval harness with at least 15 cases per builtin reviewer, performance target (§16), GoReleaser, container image, CI integration docs for GitHub Actions and GitLab CI.
- A sample Go plugin round-trips through the protocol.
- A baseline on the fixture repo makes all pre-existing findings `baseline` under `scope: repo`.
- Release artifacts build for all six OS/arch targets.

## 18. Deferred

Not in v1: a server, dashboard or database; installing analyzers; auto-committing fixes; IDE plugins; model training; Bedrock and Vertex adapters; per-sub-project profiles in monorepos; git-ref org config sources; read-only tools for agents; resolving outdated inline comments; a trends store; metric-based penalties in scoring.
