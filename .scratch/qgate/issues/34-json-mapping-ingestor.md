# 34: json_mapping ingestor

**What to build:** Bespoke analyzers with arbitrary JSON output can feed the gate.

**Blocked by:** 33 (Gate: Phase 4 acceptance)

**Status:** ready-for-agent

- [ ] JSONPath `mapping` extracts items, rule_id, message, path, start_line and severity
- [ ] Works with `import` and `run` modes and `severity_map`
- [ ] Seam C golden tests cover a representative bespoke report
