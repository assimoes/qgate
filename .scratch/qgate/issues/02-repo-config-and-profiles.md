# 02: Repo config and profiles

**What to build:** An adopting team writes a repo config and sees exactly how qgate resolves it for a stage. Covers config file discovery, schema validation with precise errors, semantic checks, profile inheritance (including abstract profiles), `--set` and env overrides, interpolation in marked fields only, and `qgate validate --stage`.

**Blocked by:** 01 (Walking skeleton)

**Status:** resolved

- [x] Repo config is discovered via flag, `QGATE_CONFIG`, then `.qgate.yaml` / `.qgate.yml`; flags beat env
- [x] `qgate validate --stage staging` prints the resolved config for the design doc's reference config without network access, given the environment variables it interpolates (an unset one is a config error)
- [x] Invalid configs exit 2 with file, dotted path and a readable message
- [x] Semantic checks reject unknown provider/agent/category references, duplicate input names, invalid globs and agent `kind` mismatches (exit 2)
- [x] `extends` resolves single inheritance; a cycle exits 2; an abstract profile can be selected with `--stage`
- [x] `--set path=value` overrides apply last; list merge rules (inputs by name, review rules by id, suppressions appended, agents deep-merged) are honored
- [x] `${VAR}` interpolation works only in schema-marked fields; API keys are only ever referenced by `api_key_env`
