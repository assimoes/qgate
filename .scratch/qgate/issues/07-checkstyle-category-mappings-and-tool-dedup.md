# 07: Checkstyle, category mappings and tool dedup

**What to build:** Teams can feed Checkstyle-format analyzers, map their vocabulary onto qgate's, and never see one issue counted twice.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope)

**Status:** ready-for-agent

- [ ] Checkstyle XML is ingested (Seam C golden tests over golangci-lint and ESLint output)
- [ ] Per-input `severity_map` applies before builtin defaults
- [ ] `category_mappings` apply in order, first match wins; else `category_default`; else `quality`
- [ ] Findings from different inputs with identical fingerprints or identical `(path, start_line, CWE set)` merge: highest severity survives, ties go to the alphabetically first input name, the rest are listed as merged sources
- [ ] Dedup output is identical regardless of input order
