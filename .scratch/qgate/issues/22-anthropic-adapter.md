# 22: Anthropic adapter

**What to build:** Teams can use Anthropic as a provider.

**Blocked by:** 19 (Provider tracer: fake, OpenAI-compatible, structured output, doctor probes)

**Status:** ready-for-agent

- [ ] Structured output via a forced tool call whose input schema is the output schema
- [ ] Reports usage and the provider-reported model id
- [ ] Works with retries and concurrency limits
- [ ] Tested against recorded or stubbed HTTP
