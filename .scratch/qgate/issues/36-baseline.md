# 36: Baseline

**What to build:** Teams adopting qgate on a legacy codebase accept existing debt so only fresh problems count.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] `qgate baseline create --output <file>` runs inputs only (no agents) with `scope: repo` and writes sorted fingerprints with rule IDs and paths
- [ ] Findings whose fingerprint is in the configured baseline become `baseline` and never score
- [ ] On the fixture repo, a baseline makes all pre-existing findings `baseline` under `scope: repo`
