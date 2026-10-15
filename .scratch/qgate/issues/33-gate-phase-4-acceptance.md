# 33: Gate: Phase 4 acceptance

**What to build:** Verify Phase 4 is complete before any Phase 5 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 25 (First reviewer end to end), 26 (Agent merging), 27 (Reviewer context and chunking), 28 (Prompt customization and review rules), 29 (Agent failures and budgets), 30 (Egress controls), 31 (Agent cache), 32 (Triager)

**Status:** ready-for-agent

- [ ] Wrong paths, out-of-diff lines, unknown categories, unknown review rule IDs and oversized suggestions are all `invalid` with reasons
- [ ] An agent finding overlapping a tool finding is `superseded` with its text as an annotation; agent-vs-agent duplicates supersede the weaker one
- [ ] A finding citing `no-raw-sql` has rule ID `review:no-raw-sql` and the review rule's severity and category
- [ ] Agent penalty respects `agent_max_penalty`; triage never changes any score
- [ ] A second run on the same commit is served fully from cache
- [ ] A provider failure under blocking enforcement with default `on_agent_error: warn` yields `warn`, exit 0 and a degraded notice
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
