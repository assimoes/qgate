# 26: Agent merging

**What to build:** Analyzers win conflicts and nothing is counted twice when agents overlap tools or each other.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] An agent finding at the same path, overlapping range and same category as a new tool finding becomes `superseded`; its message becomes an annotation and its suggestion is attached if the tool finding has none
- [ ] Agent-vs-agent duplicates keep higher severity then higher confidence; the other is `superseded` with an annotation
- [ ] Superseded findings never score
