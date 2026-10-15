# 31: Agent cache

**What to build:** Re-running the same commit is fast, cheap and gives the same agent results.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] The cache key covers agent name, prompt hash, provider type, model id, unit content, matching review rules, existing tool findings and schema version
- [ ] Hits are reported per agent; entries older than 30 days are pruned at startup
- [ ] A second run on the same commit is served fully from cache with zero provider calls
