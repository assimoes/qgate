# 17: doctor: git, inputs and VCS token scopes

**What to build:** `qgate doctor` tells an adopting team what is wrong with their setup before a real run fails.

**Blocked by:** 16 (GitLab comments)

**Status:** ready-for-agent

- [ ] Reports git context (base/head resolvable, depth sufficient)
- [ ] Reports presence of each `import` input's file
- [ ] Checks VCS token permissions and prints the minimum required scopes for GitHub and GitLab
- [ ] Exits non-zero when a check fails, with an actionable message per failure
