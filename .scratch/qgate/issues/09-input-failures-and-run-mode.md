# 09: Input failures and run mode

**What to build:** Inputs can run commands directly, and a broken analyzer is handled according to policy instead of silently passing or wrongly failing the gate.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** ready-for-agent

- [ ] `mode: run` executes an argv (no shell) in the repo root with a timeout and only the `env` allowlist, then parses its output
- [ ] A missing file, failing command, timeout or parse error marks the input `failed` in the report
- [ ] A `required` input failure, or any failure under `on_input_error: fail`, writes reports then exits 3
- [ ] Otherwise the run is degraded, the outcome is at most `warn` for that reason, and categories only that input fed become not assessed
- [ ] Infrastructure failures never exit 1
