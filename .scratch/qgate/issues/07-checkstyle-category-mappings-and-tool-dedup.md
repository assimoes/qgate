# 07: Checkstyle, category mappings and tool dedup

**What to build:** Teams can feed Checkstyle-format analyzers, map their vocabulary onto qgate's, and never see one issue counted twice.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope)

**Status:** ready-for-agent

- [ ] Checkstyle XML is ingested (Seam C golden tests over golangci-lint and ESLint output)
- [x] Per-input `severity_map` applies before builtin defaults
- [x] `category_mappings` apply in order, first match wins; else `category_default`; else `quality`
- [x] Findings from different inputs with identical fingerprints or identical `(path, start_line, CWE set)` merge: highest severity survives, ties go to the alphabetically first input name, the rest are listed as merged sources
- [x] Dedup output is identical regardless of input order

## Open

- The Seam C goldens run against hand-written fixtures in golangci-lint's and ESLint's checkstyle shape, not captured output (neither linter was installed). Replace `internal/ingest/checkstyle/testdata/{golangci,eslint}.xml` with real output and run `go test ./internal/ingest/checkstyle -update`, then tick the first box.
