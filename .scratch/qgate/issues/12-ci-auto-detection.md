# 12: CI auto-detection

**What to build:** qgate finds base, head and PR number by itself on GitHub Actions and GitLab CI, and explains how to fix a shallow clone.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope)

**Status:** ready-for-agent

- [ ] GitHub Actions: head, base and PR number come from the env and event payload as specified
- [ ] GitLab CI: head, base and MR IID come from the predefined variables
- [ ] Flags override detection; outside CI the fallback is used
- [ ] A base commit missing locally produces an error naming `fetch-depth: 0` or `GIT_DEPTH: 0`, exit 3
