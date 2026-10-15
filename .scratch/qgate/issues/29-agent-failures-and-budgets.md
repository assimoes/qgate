# 29: Agent failures and budgets

**What to build:** LLM trouble and cost are bounded and visible, and never block merges unless the team opts in.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] A provider failure under default `on_agent_error: warn` yields a degraded run, outcome at most `warn`, exit 0; `on_agent_error: fail` exits 3
- [ ] With under 10% of the global deadline left, pending units are cancelled and the run is degraded
- [ ] `max_input_tokens_per_run` is estimated before each call; units that would exceed it are skipped and listed
- [ ] `max_units_per_agent` prioritizes `priority_paths` then change size; the rest are skipped and listed
- [ ] Token usage is reported per agent and provider; cost only with a configured `pricing` table
