# 23: Secret redactor and redacting log handler

**What to build:** Secrets never appear in qgate's logs or errors, and the redactor that will scrub content sent to models exists and is proven against a corpus.

**Blocked by:** 19 (Provider tracer: fake, OpenAI-compatible, structured output, doctor probes)

**Status:** ready-for-agent

- [ ] The redactor replaces builtin patterns (cloud keys, private key blocks, JWTs, high-entropy key assignments) and `llm.redact_patterns` with `[REDACTED:<type>]`
- [ ] It removes every entry in its test corpus
- [ ] A redacting log handler wraps the configured one; tests assert no API key appears in captured logs at any level, including `-v`
