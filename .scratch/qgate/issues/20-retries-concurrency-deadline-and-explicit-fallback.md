# 20: Retries, concurrency, deadline and explicit fallback

**What to build:** Provider calls survive rate limits and slow models without hanging CI or silently switching models.

**Blocked by:** 19 (Provider tracer: fake, OpenAI-compatible, structured output, doctor probes)

**Status:** ready-for-agent

- [ ] Retries on 429, 408, 5xx and network errors with exponential backoff and full jitter (base 1s, max 30s), honoring `Retry-After`, at most 4 attempts
- [ ] `max_concurrency` is enforced per provider
- [ ] `request_timeout` applies per call alongside the global deadline
- [ ] No silent fallback; an explicit `fallback: <provider>/<model>` is used only when configured and recorded
