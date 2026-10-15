# qgate — AI-assisted quality gate CLI

Status: ready-for-agent
Technical contract: `docs/design.md`. Vocabulary: `GLOSSARY.md`. See ADR-0001.

## Problem Statement

Adopting teams already run analyzers (linters, SAST, coverage, dependency scanners) in CI, but each analyzer reports in its own format, with its own severities, its own exit-code semantics and no notion of "what did this change introduce". There is no single, stage-aware answer to "may this change merge?", so teams either ignore analyzer output or block merges on legacy debt. LLM code review is attractive, but teams cannot let a model's opinion decide merges: it is non-reproducible, drifts with model and prompt changes, and can be manipulated by the code under review. Meanwhile the platform team has no way to enforce org-wide policy (e.g. "local models only", a fixed scoring policy) across repositories.

## Solution

qgate is a single static Go binary that runs inside a CI job. It collects results from the adopting team's existing analyzers through configured inputs, optionally runs reviewer and triager agents on the change using any provider the team chooses (hosted or self-hosted), normalizes everything into one finding model, excuses findings outside the gating scope, in the baseline or suppressed, computes deterministic category and overall scores, evaluates the stage's gate, and reports the gate outcome as PR comments, machine-readable artifacts and the process exit code. Configuration is layered org config → repo config → profile → flags, with locked keys letting the platform team enforce policy. Every run's report is sufficient to reproduce its scores and gate outcome without calling any analyzer or LLM.

## User Stories

### Running the gate

1. As an adopting team, I want to run `qgate run --stage <stage>` as one CI step, so that a single exit code tells the pipeline whether the change may merge.
2. As an adopting team, I want qgate to ship as one static binary with no runtime dependency other than `git`, so that it runs in any CI system and container.
3. As an adopting team, I want qgate to auto-detect base SHA, head SHA and PR number on GitHub Actions and GitLab CI, so that I don't wire them by hand.
4. As an adopting team, I want to override base, head and PR via flags, so that I can run qgate in unusual pipelines or locally.
5. As an adopting team, I want a clear error naming the required checkout depth when the base commit is missing from a shallow clone, so that I can fix my pipeline in one step.
6. As an adopting team, I want exit code 1 to mean only "gate failed under blocking enforcement", so that infrastructure problems are never mistaken for code quality failures.
7. As an adopting team, I want exit code 2 for configuration and usage errors and exit code 3 for fatal runtime errors, so that I can tell misconfiguration from broken infrastructure.
8. As an adopting team, I want a global `--timeout` deadline, so that qgate never hangs a pipeline.
9. As an adopting team, I want qgate to warn when no repo config exists and only builtin policy is active, so that I know my policy isn't applied.

### Configuration

10. As an adopting team, I want to describe inputs, agents, review rules, suppressions, profiles and reporting in one `.qgate.yaml`, so that my policy lives with my code.
11. As an adopting team, I want `qgate validate --stage <stage>` to print the fully resolved config without network access, so that I can see exactly what a run would do.
12. As an adopting team, I want validation errors to name the file, the dotted path and a readable message, so that I can fix config mistakes quickly.
13. As an adopting team, I want semantic checks (unknown providers, unknown agents, unknown categories, `extends` cycles, duplicate input names, invalid globs, agent kind mismatches) at load time, so that bad config fails fast with exit 2.
14. As an adopting team, I want profiles to inherit from one another via `extends`, so that staging and prod can tighten a shared base without repetition.
15. As an adopting team, I want abstract profiles that exist only to be extended, so that shared defaults don't need to be a real stage.
16. As an adopting team, I want `--set` dotted-path overrides and `QGATE_CONFIG` / `QGATE_STAGE` env vars, so that a pipeline can tweak policy without editing the file.
17. As an adopting team, I want `${VAR}` interpolation only in fields marked interpolatable, so that environment-specific URLs and paths work without opening an injection hole.
18. As a platform team, I want to publish an org config by path or HTTPS URL, so that every repository starts from the same policy.
19. As an adopting team, I want to pin a URL org config with a SHA-256 checksum, so that a changed org config can't silently alter my gate.
20. As a platform team, I want to declare locked keys in the org config, so that no repo config, profile or flag can change, for example, `llm.allowed_providers` or `scoring`.
21. As an adopting team, I want list-valued config merged predictably (inputs by name, review rules by ID, suppressions appended, agents deep-merged by name), so that layering behaves as I expect.

### Inputs and analyzers

22. As an adopting team, I want to import SARIF 2.1.0 reports, so that most modern analyzers work without custom glue.
23. As an adopting team, I want to import Checkstyle XML, so that golangci-lint, ESLint and similar analyzers work.
24. As an adopting team, I want to import Cobertura and LCOV coverage, so that I get `coverage.line` and `coverage.branch` metrics at repo and diff metric scope.
25. As an adopting team, I want to import JUnit results, so that I get `tests.total`, `tests.failed` and `tests.skipped` metrics.
26. As an adopting team, I want to map arbitrary JSON output to findings with JSONPath (`json_mapping`), so that bespoke analyzers can feed the gate.
27. As an adopting team, I want an input to run a command (argv, no shell, with timeout and an env allowlist) and parse its output, so that I don't need a separate pipeline step for simple analyzers.
28. As an adopting team, I want to integrate any executable through the exec plugin protocol, so that I can add checks such as license scanning in any language.
29. As an adopting team, I want inputs marked `required`, so that a missing critical analyzer report fails the run with exit 3 instead of silently passing.
30. As an adopting team, I want non-required input failures to produce a degraded run with outcome at most `warn` for that reason, so that a flaky analyzer doesn't block merges by default.
31. As an adopting team, I want per-input severity maps and default categories, so that each analyzer's vocabulary maps onto qgate's.
32. As an adopting team, I want ordered category mappings (first match wins, e.g. anything with a CWE is security), so that findings land in the right category regardless of which analyzer produced them.
33. As an adopting team, I want unmapped severities to default to `medium` with a warning, so that a new analyzer level doesn't crash the run.

### Normalization and excusal

34. As an adopting team, I want every rule ID namespaced by its input or agent, so that rules from different analyzers never collide.
35. As an adopting team, I want fingerprints that survive inserting lines above a finding, so that baselines, suppressions and comment dedup stay stable as code moves.
36. As an adopting team, I want only findings within the gating scope (`diff`, `files` or `repo`) to be new, so that legacy debt doesn't block my merges.
37. As an adopting team, I want findings outside the gating scope reported as `out_of_scope` but never scored, so that I still see them without being blocked.
38. As an adopting team adopting qgate on a legacy codebase, I want `qgate baseline create` to record current fingerprints, so that existing debt is accepted and only fresh problems count.
39. As an adopting team, I want config suppressions by rule glob and path glob with a mandatory reason and optional expiry, so that accepted exceptions are documented and revisited.
40. As an adopting team, I want expired suppressions ignored and listed as warnings, so that exceptions don't become permanent by accident.
41. As a developer, I want inline `qgate:ignore <rule-glob> -- <reason>` comments on or above a line, so that I can excuse a specific false positive next to the code.
42. As a developer, I want inline suppressions without a reason to be ignored and reported, so that nobody silences findings without explanation.
43. As an adopting team, I want duplicate tool findings from different inputs merged deterministically (highest severity survives; ties to the alphabetically first input name; others recorded as merged sources), so that one issue isn't counted twice and the report is reproducible.

### Agents

44. As an adopting team, I want to configure reviewers with a provider, model, categories and prompt, so that LLM review covers areas my analyzers miss.
45. As an adopting team, I want to configure a triager that annotates existing tool findings with a verdict (`likely_true`, `likely_false_positive`, `uncertain`) and an explanation, so that reviewers can dismiss noise faster.
46. As an adopting team, I want every agent to declare `kind: reviewer` or `kind: triager`, so that its role is explicit and mismatched prompts are rejected.
47. As an adopting team, I want builtin versioned prompts (`security-reviewer@1`, `quality-reviewer@1`, `triage@1`), so that I get useful review with zero prompt engineering.
48. As an adopting team, I want to extend or replace the role prompt with my own templated files, so that agents know my codebase's conventions.
49. As a platform team, I want the core preamble and output contract to be impossible to remove, so that customization never breaks parsing or injection defenses.
50. As an adopting team, I want to write review rules in natural language with a severity, category, path globs and target agents, so that house rules like "no raw SQL" are enforced.
51. As an adopting team, I want findings that enforce a review rule to carry rule ID `review:<review-rule-id>` and take the review rule's severity and category, so that I can suppress, baseline and gate on review-rule violations like any other rule.
52. As an adopting team, I want `qgate prompts show <agent>` to print the fully assembled system prompt for a stage, so that I can see what the model is told.
53. As an adopting team, I want reviewers to see line-numbered changed hunks with surrounding context, so that their line references are accurate.
54. As an adopting team, I want reviewers to be told the existing tool findings for a file, so that they don't repeat what analyzers already found.
55. As an adopting team, I want large files split into multiple units of work at hunk boundaries to fit the model's context window, so that big changes are still reviewed.
56. As an adopting team, I want triager units to be batches of at most `max_findings_per_unit` findings (default 10) from a single file, so that triage calls stay coherent and bounded.
57. As an adopting team, I want agent output validated (path matches the unit, lines within the change, category allowed, severity clamped, suggestion sane, count capped), with failures kept as `invalid` with a reason, so that hallucinated findings never score.
58. As an adopting team, I want an agent finding that overlaps a tool finding in the same category to become `superseded` with its text attached as an annotation, so that analyzers win conflicts.
59. As an adopting team, I want duplicate agent findings resolved by keeping the higher severity then higher confidence and superseding the other, so that one issue isn't counted twice.
60. As an adopting team, I want agent results cached by a content-addressed key, so that re-running the same commit is fast, cheap and stable.
61. As an adopting team, I want `--no-llm` to guarantee zero calls to LLM providers, so that I can run a deterministic-only gate.
62. As an adopting team, I want agent failures to produce a degraded run and `warn` rather than `fail` by default, so that an LLM outage never blocks merges unless I opt in with `on_agent_error: fail`.

### LLM providers

63. As an adopting team, I want to use Anthropic, OpenAI, Azure OpenAI or any OpenAI-compatible endpoint (Ollama, vLLM, LM Studio, llama.cpp, LiteLLM), so that I choose hosted or self-hosted models.
64. As an adopting team, I want structured output to work even on models without native JSON-schema support (prompted schema plus one repair attempt), so that local models are usable.
65. As an adopting team, I want per-provider concurrency limits, request timeouts and retries with backoff that honor `Retry-After`, so that rate limits don't fail the run.
66. As an adopting team, I want no silent fallback to another provider or model, and any configured fallback recorded in the report, so that I always know which model reviewed my code.
67. As an adopting team, I want token budgets per run and unit caps per agent, with units prioritized by risk (priority paths, then change size; triager units by severity) and skipped units listed, so that cost is bounded and predictable.
68. As an adopting team, I want token usage reported per agent and provider, and cost only when I configure a price table, so that qgate never shows stale prices.
69. As an adopting team, I want `qgate doctor` to probe connectivity, model existence, structured-output support and context window for each provider, so that I can debug a self-hosted model before it breaks CI.

### Scoring and gating

70. As an adopting team, I want category and overall scores computed by a pure, versioned, integer-arithmetic function of new findings, so that scores are identical across platforms and reruns (ADR-0001).
71. As a platform team, I want agent influence on scores bounded by `agent_weight_pct`, `agent_min_confidence` and `agent_max_penalty`, so that a manipulated or noisy agent can never pass a failing change and can cost only bounded points.
72. As an adopting team, I want categories no successful input or agent covered marked `not_assessed` and excluded from the overall score, so that a missing analyzer never produces a perfect 100.
73. As an adopting team, I want gate checks for overall minimum, per-category minimum, maximum new findings by severity (all or tool-only), required assessed categories and metric thresholds, so that I can express stage-specific quality bars.
74. As an adopting team, I want advisory enforcement for early stages and blocking enforcement for later ones, so that teams see the gate before it bites.
75. As an adopting team, I want the gate result to list every gate check with expected vs. actual and pass/fail/skipped, so that I know exactly why a gate failed.
76. As an adopting team, I want `qgate score --report <file>` to reproduce scores and gate outcome exactly from a saved report without calling any analyzer or LLM, so that any gate decision can be audited.

### Reporting and PR integration

77. As a developer, I want console output with the gate outcome first, then category scores, failed gate checks and top findings, so that the CI log answers "why" at a glance.
78. As an adopting team, I want a canonical JSON report with run metadata, resolved config (no secrets), inputs, agents, findings, metrics, scores, gate and data egress, so that dashboards and audits can consume it.
79. As an adopting team, I want SARIF output of new findings, so that I can upload to GitHub code scanning.
80. As an adopting team, I want a Markdown artifact identical to the PR summary, so that the summary is available even without PR comments.
81. As a developer, I want one summary comment per PR and stage that is edited on every run, so that my PR isn't flooded with bot comments.
82. As a developer, I want inline comments on new findings at or above a configured severity, capped in count, so that I see problems on the lines that cause them.
83. As a developer, I want inline comments carrying one-click suggestion blocks when the replacement lies entirely within the diff, so that I can accept fixes quickly.
84. As a developer, I want inline comments never re-posted for a fingerprint that already has one, so that reruns don't duplicate comments.
85. As a developer, I want findings that can't be positioned in the diff listed in the summary instead, so that nothing is silently dropped.
86. As a developer, I want `likely_false_positive` triage verdicts shown prominently with a ready-to-copy inline suppression line, so that I decide quickly while a human stays in control.
87. As an adopting team, I want inline comments posted as a plain review (never "request changes"), so that qgate's exit code, not a review state, is what blocks.
88. As an adopting team, I want GitHub Enterprise Server and self-managed GitLab supported via an API URL override, so that qgate works on our hosting.
89. As an adopting team, I want reporter failures never to change the gate outcome (except failing to write the JSON report, which is exit 3), so that a comment API hiccup doesn't block a merge.
90. As an adopting team, I want `qgate doctor` to check VCS token permissions and print the minimum required scopes, so that comment posting works first time.

### Security and privacy

91. As a platform team, I want repository content treated strictly as untrusted data inside delimited blocks, so that code under review can't instruct agents.
92. As a platform team, I want agent text escaped in PR comments (no raw HTML, `@mentions` or qgate markers), so that a manipulated agent can't spoof or ping people.
93. As a platform team, I want secrets redacted from content before any LLM call, with builtin and custom patterns, so that credentials in diffs never leave CI.
94. As a platform team, I want `llm.exclude_paths` never sent to any provider, so that sensitive files stay private.
95. As a platform team, I want `llm.allowed_providers` lockable, so that I can enforce "local models only".
96. As a platform team, I want API keys referenced only by environment variable name and never written to logs, reports or comments at any log level, so that secrets never leak.
97. As a platform team, I want the report to list which files were sent to which provider, so that data egress is auditable.
98. As an adopting team, I want prompts and raw model responses written only under `.qgate/debug/` and only with `--debug-prompts`, so that debugging is possible without leaking code into logs.

### Evaluation and operations

99. As a qgate maintainer, I want `qgate eval --suite <dir>` to report precision, recall, invalid-output rate and token usage per agent against labeled cases, so that prompt and model changes are measured.
100. As a qgate maintainer, I want builtin prompts immutable once released, with any change shipping as a new version plus a recorded eval run, so that adopting teams' results don't shift underneath them.
101. As an adopting team, I want `qgate version` to show version, commit, build date and scoring algorithm version, so that I can correlate behavior with releases.
102. As an adopting team, I want per-stage timings in the report and structured `slog` logs in text or JSON, so that I can see where CI time goes.
103. As an adopting team, I want a deterministic-only run with 5,000 findings to finish under 5 seconds and 500 MB, so that qgate never becomes the slow step.
104. As an adopting team, I want release binaries for linux/darwin/windows × amd64/arm64 and a distroless container image, so that I can use qgate anywhere.

## Implementation Decisions

`docs/design.md` holds the detailed contracts (algorithms, formats, phase order). The decisions below are the load-bearing ones and are already reflected there.

### Architecture

- Pipeline order is fixed: config loader → context builder → inputs (parallel) → normalizer → gating-scope classification and excusal → agents (reviewers in parallel, then triager) → merge and dedup → scoring → gate evaluation → reporters → exit code. Only inputs and agents have nondeterministic external effects; everything after them is a pure function of findings, metrics and resolved config.
- Modules: CLI, config (load/merge/interpolate/validate/profile resolution), model, git context, ingest (registry of ingestors keyed by format), normalize, llm (provider interface, adapters, structured output), agent (runner, context assembly, validation, triage), prompts (embedded, versioned), score, gate, report, vcs (GitHub, GitLab), cache.
- The CLI exposes a single in-process entry point taking context, args, environment, working directory and output streams and returning an exit code; `main` is a thin wrapper. Provider and VCS HTTP transport are injectable through this entry point so tests can substitute the fake provider and recorded HTTP.
- No global mutable state; `context.Context` through every I/O call; `log/slog` with a redacting handler wrapping the configured one.
- Third-party dependencies limited to design §3.2; any addition is logged in `docs/DECISIONS.md`.
- Decision records: architectural, hard-to-reverse decisions go in `docs/adr/`; the implementer's running "simplest reasonable choice" log goes in `docs/DECISIONS.md`.

### Domain model and wire format (design §6)

- Finding statuses: `new`, `out_of_scope` (renamed from `outside_diff`), `baseline`, `suppressed`, `superseded`, `invalid`. Only `new` findings score or count toward gate checks.
- `superseded` covers both agent-vs-tool overlap (tool wins) and agent-vs-agent duplicates (higher severity, then higher confidence wins). The superseded finding's message becomes an annotation on the winner; its suggestion is attached if the winner has none.
- Annotations are agent commentary only: agent name, optional verdict (triage), explanation. Tool-to-tool dedup no longer uses annotations; the surviving finding carries a separate **merged sources** list (input name, rule ID, original severity, fingerprint of each folded finding).
- Tool-to-tool dedup: findings with identical fingerprints, or identical `(path, start_line, CWE set)`, from different inputs merge. The survivor is the highest severity; ties break on the alphabetically first input name. The survivor keeps its own fingerprint, rule ID, title and message.
- Rule ID namespaces: `<input-name>:<rule>` for tool findings; `agent:<agent-name>:<slug>` for agent findings not tied to a review rule; `review:<review-rule-id>` for agent findings enforcing a review rule.
- Severity serializes as lowercase strings; confidence is an integer 0–100; everything in the report is versioned by `schema_version`.

### Configuration (design §5)

- Key renames: profile `mode` → `enforcement` (`advisory | blocking`); top-level `rules` → `review_rules`; `category_rules` → `category_mappings`. Input `mode` (`import | run | exec_plugin`) is unchanged.
- Every agent declares `kind: reviewer | triager`. A builtin prompt base whose kind differs from the agent's kind is a config error. Triagers use `applies_to` and `max_findings_per_unit` (input batch size, default 10); reviewers use `categories` and `max_findings_per_unit` (output cap).
- Profiles may be abstract (only extended); `--stage` naming an abstract profile is permitted.
- Merge semantics, locked keys, `extends` single inheritance with cycle detection, interpolation restrictions and file discovery are as in design §5.
- JSON Schema 2020-12 is the source of truth for config shape; YAML decodes to a generic map, validates, then decodes to typed structs.

### Review rules

- Review rules are rendered into the prompts of matching reviewers (by path glob and target agents) as a numbered list with their IDs.
- The reviewer output contract gains an optional `review_rule_id`. When present and valid for that unit, the finding's rule ID becomes `review:<id>` and its severity and category are taken from the review rule, overriding the model. An unknown or non-matching `review_rule_id` makes the finding `invalid`.

### Agents and units of work

- A unit of work is one LLM call's input. Reviewer units are one changed file's hunks (split at hunk boundaries to fit 60% of the context window). Triager units are batches of up to `max_findings_per_unit` new tool findings above `applies_to.min_severity`, never mixing files.
- Budgets (`max_input_tokens_per_run`, `max_units_per_agent`) apply to both kinds. Reviewer units are prioritized by priority paths then change size; triager units by highest severity first. Skipped units are listed in the report.
- Triagers never change status, severity or score.
- Prompt assembly order, untrusted-input handling, output validation and caching are as in design §11.

### Scoring and gate

- Scoring algorithm v1 as in design §12.2, in integer hundredths, order-independent, frozen once released. LLMs never produce scores (ADR-0001).
- Gate checks and outcome logic as in design §13; terminology is "gate check" (not "rule") throughout the report and PR summary.

### Reporting, VCS, security

- As in design §14–§15, with report and summary vocabulary updated to the glossary (gate checks, enforcement, out of scope, merged sources).

## Testing Decisions

- A good test exercises observable behavior through a public seam (exit code, report contents, rendered Markdown, recorded HTTP traffic, function return values of the pure seams) and never asserts on internal package structure, intermediate types or call order. Tests should survive any refactor that preserves behavior.
- **Seam A — CLI entry point (primary).** Tests invoke the in-process CLI entry point against a real `git` fixture repository built per test (commits created programmatically: added, modified, renamed, deleted files), fixture analyzer outputs on disk, the `fake` LLM provider (canned responses keyed by request hash; can simulate invalid JSON, timeouts, 429s and repair success) and go-vcr recorded VCS HTTP. Assertions are on exit code, the JSON report, the Markdown summary and recorded requests. This seam covers config layering, locked keys, validation messages, gating scope, baseline, suppressions, dedup, agent validation and merging, structured-output repair, retries, budgets, caching, degraded runs, exit codes, PR comment upsert and dedup, comment escaping, secret redaction (including asserting no API key appears in captured logs at any level), `--no-llm` making zero provider calls, and `qgate score --report` reproducing `scores` and `gate` byte-for-byte.
- **Seam B — scoring and gate evaluation as pure functions.** Golden test for the design §12.3 worked example (security score 76). Property tests: order independence; adding a new finding never increases any score; excused findings never change scores; agent penalty never exceeds `agent_max_penalty` per category; gate outcome is a function of its inputs alone.
- **Seam C — ingestor parse per format.** Golden-file tests (regenerable with `-update`) over real outputs from Semgrep, golangci-lint (Checkstyle and SARIF), gosec, Trivy, CodeQL, ESLint, Cobertura, LCOV and JUnit, asserting normalized findings and metrics.
- No other seams. Normalizer steps, prompt assembly, the redactor and provider adapters are tested through Seam A (provider adapters against a local stub server via the same injected transport).
- Agent quality is measured, not unit-tested: the eval harness with at least 15 labeled cases per builtin reviewer.
- Prior art: none — the repository is greenfield. Design §16 is the reference.
- `go vet`, `staticcheck` and `go test -race` stay green at the end of every phase.

## Out of Scope

- A server, dashboard or database; the JSON report is the integration point.
- Installing or version-managing analyzers.
- Automatically committing fixes; suggestions are review comments only.
- IDE plugins.
- Training or fine-tuning models.
- Bedrock and Vertex adapters (must fit the provider interface later).
- Monorepo per-sub-project profiles, git-ref org config sources, read-only agent tools, resolving outdated inline comments, a trends store, and metric-based penalties in scoring (design §18).

## Further Notes

- Deliver in the design §17 phase order (0 Skeleton → 1 Deterministic gate → 2 PR reporting → 3 LLM provider layer → 4 Agents → 5 Extensibility and hardening); each phase's acceptance criteria must pass in CI before the next starts.
- `GLOSSARY.md` is the canonical vocabulary.
- Rationale for deterministic scoring: `docs/adr/0001-llms-produce-findings-never-scores.md`.
