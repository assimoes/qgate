# 32: Triager

**What to build:** A `kind: triager` agent helps developers dismiss noise faster without ever deciding anything itself.

**Blocked by:** 26 (Agent merging)

**Status:** ready-for-agent

- [ ] Units are batches of up to `max_findings_per_unit` (default 10) new tool findings at or above `applies_to.min_severity`, never mixing files, prioritized by severity under budgets
- [ ] Output becomes annotations (verdict, explanation) and may add a suggestion to a finding that has none
- [ ] Triagers never change status, severity or any score
- [ ] Draft builtin `triage@1` prompt
- [ ] `likely_false_positive` verdicts appear prominently in the PR summary and Markdown with a ready-to-copy inline suppression line
