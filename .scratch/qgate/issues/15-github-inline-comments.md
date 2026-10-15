# 15: GitHub inline comments

**What to build:** Developers see new findings on the exact lines that caused them, with one-click suggestions where possible, and reruns never duplicate comments.

**Blocked by:** 14 (GitHub summary comment)

**Status:** ready-for-agent

- [ ] New findings at or above `inline_min_severity` are posted, ordered by severity, capped at `max_inline_comments`
- [ ] Posted as one review with event `COMMENT`, never `REQUEST_CHANGES`
- [ ] Each comment carries its `qgate:fp=<fingerprint>` marker; a second run posts no duplicates
- [ ] Suggestion blocks appear only when the replacement range lies entirely in the diff
- [ ] Findings that cannot be positioned in the diff are listed in the summary instead
