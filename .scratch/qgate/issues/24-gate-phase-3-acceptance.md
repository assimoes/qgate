# 24: Gate: Phase 3 acceptance

**What to build:** Verify Phase 3 is complete before any Phase 4 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 19 (Provider tracer: fake, OpenAI-compatible, structured output, doctor probes), 20 (Retries, concurrency, deadline and explicit fallback), 21 (OpenAI and Azure OpenAI adapters), 22 (Anthropic adapter), 23 (Secret redactor and redacting log handler)

**Status:** ready-for-agent

- [ ] Structured output works natively and via prompt + repair (fake provider)
- [ ] Retries honor `Retry-After` and stop after 4 attempts
- [ ] The redactor removes its whole corpus; no API key appears in any log line at any level
- [ ] `qgate doctor` reports probe results for an OpenAI-compatible stub server
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
