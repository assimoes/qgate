# 14: GitHub summary comment

**What to build:** On a GitHub PR, qgate keeps one summary comment per stage up to date with the gate result.

**Blocked by:** 13 (Gate: Phase 1 acceptance)

**Status:** ready-for-agent

- [ ] The summary is found by its `qgate:summary stage=<stage>` marker and edited on each run; different stages get separate comments
- [ ] Content order matches the design doc (outcome, scores, failed gate checks, counts, top findings with links, degraded notices, agents, job link)
- [ ] Agent-like text containing `<!--`, `@user` and raw HTML is escaped
- [ ] `--no-comments` disables it; a VCS API failure does not change the exit code
- [ ] `QGATE_VCS_TOKEN` falls back to `GITHUB_TOKEN`; GitHub Enterprise works via `reporting.pr.api_url`
- [ ] Tested with go-vcr recorded HTTP
