# 06: Scores, gate checks, enforcement and score --report

**What to build:** qgate turns new findings into category and overall scores, evaluates the profile's gate and exits with the right code; any saved report reproduces its result exactly.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope)

**Status:** ready-for-agent

- [ ] Scoring v1 is a pure, integer, order-independent function; the design doc's worked example scores exactly 76 (Seam B golden test)
- [ ] Seam B property tests: order independence; adding a new finding never raises a score; excused findings never change scores; agent penalty ≤ `agent_max_penalty`
- [ ] Categories with no successful input are not assessed (score `null`, excluded from overall); no assessed categories → overall `null`
- [ ] Gate checks `overall_min`, `category_min`, `max_new`, `max_new_tool`, `require_assessed` evaluate as specified, each listed with expected, actual and pass/fail/skipped
- [ ] Blocking enforcement with a violated gate check exits 1; advisory exits 0 with `warn`
- [ ] `qgate score --report` reproduces the `scores` and `gate` sections byte-for-byte without running inputs
