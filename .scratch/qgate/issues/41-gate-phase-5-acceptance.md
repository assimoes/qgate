# 41: Gate: Phase 5 acceptance

**What to build:** Verify Phase 5 and v1 are complete. No new behavior; fix whatever the checks reveal.

**Blocked by:** 34 (json_mapping ingestor), 35 (Exec plugins), 36 (Baseline), 37 (Inline suppressions), 38 (Eval harness and builtin prompts v1), 39 (Performance target), 40 (Release and CI docs)

**Status:** ready-for-agent

- [ ] A sample Go plugin round-trips through the protocol
- [ ] A baseline on the fixture repo makes all pre-existing findings `baseline` under `scope: repo`
- [ ] Release artifacts build for all six OS/arch targets
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
