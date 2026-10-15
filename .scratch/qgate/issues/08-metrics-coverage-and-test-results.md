# 08: Metrics: coverage and test results

**What to build:** Coverage and test-result reports become metrics the gate can check.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** ready-for-agent

- [ ] Cobertura and LCOV produce `coverage.line` and `coverage.branch` at `repo` metric scope and at `diff` metric scope computed from changed ranges
- [ ] JUnit produces `tests.total`, `tests.failed`, `tests.skipped`
- [ ] `metrics[]` gate checks support all six operators and fail on a missing metric unless `optional: true`
- [ ] Seam C golden tests cover real Cobertura, LCOV and JUnit output
