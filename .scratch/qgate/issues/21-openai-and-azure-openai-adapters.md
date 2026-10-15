# 21: OpenAI and Azure OpenAI adapters

**What to build:** Teams can use OpenAI or Azure OpenAI as providers.

**Blocked by:** 19 (Provider tracer: fake, OpenAI-compatible, structured output, doctor probes)

**Status:** ready-for-agent

- [ ] `openai` uses native JSON-schema output
- [ ] `azure_openai` uses the Azure base URL and api-version, with the deployment name as model
- [ ] Both report usage and the provider-reported model id
- [ ] Tested against recorded or stubbed HTTP
