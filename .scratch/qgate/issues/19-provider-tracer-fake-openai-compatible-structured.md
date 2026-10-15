# 19: Provider tracer: fake, OpenAI-compatible, structured output, doctor probes

**What to build:** An adopting team points qgate at a self-hosted OpenAI-compatible model and `qgate doctor` proves it works, including structured output. Tracer bullet for the LLM provider layer: the vendor-neutral provider interface, the `fake` provider, the `openai_compatible` adapter, the structured-output wrapper and the probes.

**Blocked by:** 18 (Gate: Phase 2 acceptance)

**Status:** ready-for-agent

- [ ] Agents-facing code depends only on the provider interface
- [ ] The `fake` provider replays canned responses by request hash and can simulate invalid JSON, timeouts, 429s and repair success
- [ ] Structured output uses native support when declared; otherwise schema-in-prompt, JSON extraction tolerant of fences and prose, validation, and exactly one repair request
- [ ] Repair failure returns an invalid-output error without crashing
- [ ] `qgate doctor` probes connectivity, model existence, structured output (native / after repair / failed) and declared vs. discovered context window against a local stub server
