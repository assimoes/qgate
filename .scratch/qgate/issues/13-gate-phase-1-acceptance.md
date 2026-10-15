# 13: Gate: Phase 1 acceptance

**What to build:** Verify Phase 1 is complete before any Phase 2 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope), 06 (Scores, gate checks, enforcement and score --report), 07 (Checkstyle, category mappings and tool dedup), 08 (Metrics: coverage and test results), 09 (Input failures and run mode), 10 (Config suppressions), 11 (File reporters: SARIF, Markdown and console), 12 (CI auto-detection)

**Status:** ready-for-agent

- [ ] Findings outside changed lines are `out_of_scope` and do not affect scores
- [ ] The worked example produces exactly 76
- [ ] Blocking with a violation exits 1; advisory exits 0 with `warn`; a missing required input with `on_input_error: fail` exits 3
- [ ] `qgate score --report` reproduces `scores` and `gate` byte-for-byte
- [ ] Fingerprints survive lines inserted above a finding; tool dedup is independent of input order
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
