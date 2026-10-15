# 18: Gate: Phase 2 acceptance

**What to build:** Verify Phase 2 is complete before any Phase 3 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 14 (GitHub summary comment), 15 (GitHub inline comments), 16 (GitLab comments), 17 (doctor: git, inputs and VCS token scopes)

**Status:** ready-for-agent

- [ ] A second run on the same PR edits the summary and posts no duplicate inline comments (recorded HTTP)
- [ ] Suggestions outside the diff are not posted as suggestion blocks
- [ ] Text containing `<!--`, `@user` and HTML is escaped in rendered comments
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
