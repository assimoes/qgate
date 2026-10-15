# 30: Egress controls

**What to build:** Platform teams control and audit exactly what code leaves CI.

**Blocked by:** 25 (First reviewer end to end)

**Status:** ready-for-agent

- [ ] Content passes the redactor before any LLM call when `llm.redact_secrets` is on (default)
- [ ] `llm.exclude_paths` files are never sent to any provider
- [ ] Profiles referencing a provider outside `llm.allowed_providers` are a config error
- [ ] The report's `data_egress` lists files sent per provider
- [ ] `--debug-prompts` writes prompts and raw responses only under the debug directory, never to stdout; without it, they are never written
