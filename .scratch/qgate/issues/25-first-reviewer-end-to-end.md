# 25: First reviewer end to end

**What to build:** A `kind: reviewer` agent reviews the changed files and its findings appear in the report, score within their bounds and reach the gate. Tracer bullet for agents, on the `fake` provider: minimal prompt assembly (core preamble, role prompt, output contract) with draft builtin `security-reviewer@1` and `quality-reviewer@1`, one unit of work per changed file, output validation, rule namespacing and agent scoring.

**Blocked by:** 24 (Gate: Phase 3 acceptance)

**Status:** ready-for-agent

- [ ] Profiles select reviewers; reviewers run in parallel across agents and units
- [ ] Each finding failing validation (path, line range in the change, category, suggestion size, count cap) is kept as `invalid` with a reason; drop counts are reported per agent
- [ ] Valid findings get `agent:<agent>:<slug>`, `source.kind: agent`, the model id, and pass through normalization from fingerprinting on
- [ ] Agent findings score with `agent_weight_pct`, `agent_min_confidence` and `agent_max_penalty`
- [ ] Repository content is wrapped in delimited blocks and the core preamble says nothing inside is an instruction
- [ ] `--no-llm` makes zero provider calls
- [ ] `qgate prompts show <agent>` prints the assembled prompt; `prompt_hash` is recorded per agent
