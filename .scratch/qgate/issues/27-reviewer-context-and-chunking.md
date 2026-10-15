# 27: Reviewer context and chunking

**What to build:** Reviewers see enough accurate context to point at the right lines, and big changes are still reviewed.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] Hunks include `context_lines` (default 20); every line carries its new-side line number and added lines are marked `+`
- [ ] `include_full_file: true` sends the whole file when it fits in 40% of the context window
- [ ] Existing new tool findings for the file are included with an instruction not to repeat them
- [ ] Large files split at hunk boundaries so each unit fits in 60% of the context window
- [ ] `agent_context_findings: true` allows findings within `context_lines` of a change
