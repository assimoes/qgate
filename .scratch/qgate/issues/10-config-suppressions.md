# 10: Config suppressions

**What to build:** Adopting teams excuse known findings from config with a documented reason and expiry.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** ready-for-agent

- [ ] Suppressions match on rule glob and path glob and mark findings `suppressed` with the reason in the status note
- [ ] Expired suppressions are ignored and listed as warnings
- [ ] Suppressed findings never change scores or gate checks
