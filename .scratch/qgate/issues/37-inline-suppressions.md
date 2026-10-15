# 37: Inline suppressions

**What to build:** Developers excuse a specific false positive right next to the code, with a reason.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] `qgate:ignore <rule-id-glob> -- <reason>` on the same line or the line above suppresses matching findings with the reason as status note
- [ ] Inline suppressions without a reason are ignored and reported
- [ ] Works for tool and agent findings, including `review:` rule IDs
