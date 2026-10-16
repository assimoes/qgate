# 05: First findings: git context, SARIF and gating scope

**What to build:** Running qgate on a real git repository with a SARIF report produces normalized findings in the report, each marked `new` or `out_of_scope` according to the gating scope. Tracer bullet for the deterministic pipeline: base/head from flags or the merge-base fallback, diff to changed line ranges, the SARIF ingestor registered by format, and normalization steps 1–6 (paths, rule namespacing, severity, default category, fingerprint, gating scope).

**Blocked by:** 04 (Gate: Phase 0 acceptance)

**Status:** resolved

- [x] A Seam A fixture-repo helper creates commits programmatically covering added, modified, renamed and deleted files
- [x] `--base`/`--head` work; without them the merge-base fallback is used
- [x] SARIF 2.1.0 with multiple runs is ingested; `level` and `security-severity` map to severity per the design doc; CWE is extracted; `partialFingerprints` are used when present
- [x] Rule IDs are namespaced `<input-name>:<rule>`; unmapped severities become `medium` with a warning
- [x] `project.include`/`exclude` are applied; binary files have no ranges
- [x] Under `diff`, findings on unchanged lines are `out_of_scope`; file-level findings are `new` only if the file changed; `files` and `repo` behave as specified
- [x] Fingerprints are unchanged when lines are inserted above a finding
- [x] Seam C golden tests (with `-update`) cover real Semgrep, CodeQL and Trivy SARIF output
