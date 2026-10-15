# 38: Eval harness and builtin prompts v1

**What to build:** Maintainers measure agent quality so prompt and model changes are evidence-based; builtin prompts are finalized at v1.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] Eval cases hold a snapshot or patch, expected findings (path, range, category, minimum severity) and known non-issues
- [ ] `qgate eval --suite <dir> --agent <name>` reports precision, recall, invalid-output rate and token usage per agent
- [ ] At least 15 cases per builtin reviewer
- [ ] `security-reviewer@1`, `quality-reviewer@1` and `triage@1` are finalized and frozen, with an eval run recorded
- [ ] Any later builtin prompt change requires a new version
