# 11: File reporters: SARIF, Markdown and console

**What to build:** Every run leaves human- and machine-readable artifacts beyond the JSON report.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** ready-for-agent

- [ ] Console shows outcome first, then category scores, failed gate checks and top findings; color only on a TTY without `NO_COLOR`
- [ ] SARIF output contains new findings only by default, one run per source
- [ ] Markdown output matches the PR summary content and is capped at 60,000 characters with a pointer to artifacts
- [ ] `--output format=path` and `reporting.outputs` both work
- [ ] A reporter failure does not change the exit code, except failing to write the JSON report (exit 3)
- [ ] Golden tests cover console (no color), Markdown and SARIF output
