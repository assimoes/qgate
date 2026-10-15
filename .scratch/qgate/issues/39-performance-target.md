# 39: Performance target

**What to build:** qgate is never the slow step in a pipeline.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] A deterministic-only run with 5,000 findings across inputs completes in under 5 seconds on a standard CI runner, excluding `run`-mode command time
- [ ] Peak memory for that workload is under 500 MB
- [ ] The benchmark runs in CI and fails on regression past the target
