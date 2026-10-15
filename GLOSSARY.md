# qgate

An in-house CLI that runs in CI, gathers findings from analyzers and LLM agents, scores them deterministically, and decides whether a change passes a stage's quality gate.

## People

**Adopting team**:
An internal team that runs qgate in its repositories and owns the repo config.
_Avoid_: Customer, user, client

**Platform team**:
The internal team that owns the org config and may lock keys to enforce policy across adopting teams.
_Avoid_: Admin, org owner

## Sources of findings

**Analyzer**:
An external, deterministic program whose results qgate consumes (e.g. Semgrep, golangci-lint, a coverage tool).
_Avoid_: Tool (for the program itself), scanner, linter (as a generic term)

**Input**:
A configured entry that brings an analyzer's results into qgate, by importing a file, running a command, or calling an exec plugin.
_Avoid_: Source, integration

**Exec plugin**:
An executable that speaks qgate's plugin protocol, used as an input.
_Avoid_: Plugin (for any other kind of extension)

**Agent**:
An LLM-backed participant inside qgate, defined by a prompt, a provider and a model. Comes in two kinds: Reviewer and Triager.
_Avoid_: Bot, AI, coding agent (that refers to whoever implements qgate)

**Reviewer**:
An agent that reads changed code and produces new findings.

**Triager**:
An agent that reads existing findings and attaches annotations; it never creates findings or changes scores.
_Avoid_: Triage agent, verifier

**Unit of work**:
The input to a single LLM call by an agent: a chunk of one changed file for a reviewer, or a batch of findings for a triager.
_Avoid_: Chunk, task, job

## Findings

**Finding**:
A located issue with a rule ID, severity and category.
_Avoid_: Issue, violation, alert

**Rule**:
The check an analyzer or agent reports a finding against, identified by a namespaced rule ID.
_Avoid_: Check (reserved for gate checks)

**Tool finding**:
A finding that came from an input.
_Avoid_: Analyzer finding, deterministic finding

**Agent finding**:
A finding produced by a reviewer.
_Avoid_: LLM finding, AI finding

**Fingerprint**:
A stable identity for a finding across runs that does not depend on line numbers.
_Avoid_: ID, hash

**New finding**:
A finding not excused by gating scope, baseline or suppression; only new findings count toward scores and the gate. It does not mean "introduced in this PR."
_Avoid_: Active finding, open finding

**Excused finding**:
A finding that is reported but never scored, because it lies outside the gating scope, is in the baseline, or is suppressed.

**Baseline**:
A recorded set of fingerprints accepted as existing debt.
_Avoid_: Allowlist, snapshot

**Suppression**:
An explicit, reasoned instruction to excuse findings matching a rule and path, declared in config or inline.
_Avoid_: Ignore, waiver, exception

**Superseded finding**:
An agent finding that duplicates a stronger finding (a tool finding, or an agent finding of higher severity then confidence); the stronger one wins and the superseded finding's text becomes an annotation on it.

**Annotation**:
Commentary an agent attaches to a finding: a triager's verdict, or a superseded agent finding's message.
_Avoid_: Note, comment

**Merged sources**:
The other inputs that reported the same issue as a finding and were folded into it during deduplication.

**Suggestion**:
A proposed replacement for a line range, posted for a human to accept; never applied automatically.
_Avoid_: Fix, autofix, patch

**Review rule**:
A natural-language requirement written by an adopting team and given to matching reviewers; findings that enforce it carry its ID, severity and category.
_Avoid_: Custom rule, rule (unqualified), policy

**Category mapping**:
A config entry that assigns a category to findings matching a pattern.
_Avoid_: Category rule

## Measurement

**Category**:
A quality dimension that findings belong to and that is scored separately (e.g. security, quality, maintainability, testing, dependencies).

**Metric**:
A numeric measurement, such as line coverage, with a metric scope.

**Metric scope**:
What a metric measures over: the diff, the whole repo, or a single file.

**Assessed category**:
A category that at least one successful input or agent covered in this run; only assessed categories get a score.
_Avoid_: Covered category

**Category score**:
A 0–100 score for one category, derived from its new findings' penalties.

**Overall score**:
The weighted average of the category scores across assessed categories.

## Gating

**Stage**:
The pipeline environment a run targets (e.g. dev, staging, prod), named with `--stage`.
_Avoid_: Environment, phase

**Profile**:
A named configuration block that a stage selects. Abstract profiles exist only to be extended.
_Avoid_: Preset, mode

**Gating scope**:
Which findings can count as new: those on changed lines, those in changed files, or all of them.
_Avoid_: Scope (unqualified)

**Enforcement**:
Whether a profile's gate can block a merge (blocking) or only report (advisory).
_Avoid_: Mode

**Gate**:
The set of gate checks a profile applies to scores, findings and metrics.
_Avoid_: Policy, quality bar

**Gate check**:
A single pass/fail condition in a gate, such as a minimum category score or a maximum count of new critical findings.
_Avoid_: Rule, gate rule

**Gate outcome**:
The result of evaluating a gate: pass, warn or fail.
_Avoid_: Verdict (reserved for triage), status

**Degraded run**:
A run in which at least one input or agent failed, or a budget or deadline was exceeded, but the gate was still evaluated.
_Avoid_: Partial run, failed run

## Configuration

**Org config**:
The platform team's configuration layer, shared across repositories.

**Repo config**:
An adopting team's configuration file in its repository.

**Locked key**:
A config path that the org config declares and that no later layer may change.
_Avoid_: Frozen key, enforced setting

**Report**:
The canonical JSON record of a run, from which scores and the gate outcome can be reproduced without calling any analyzer or LLM.
_Avoid_: Output, result file
