# 04: Gate: Phase 0 acceptance

**What to build:** Verify Phase 0 is complete before any Phase 1 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 01 (Walking skeleton), 02 (Repo config and profiles), 03 (Org config and locked keys)

**Status:** resolved

- [x] `qgate validate --stage staging` prints the resolved reference config; invalid configs error with file, path and message
- [x] Locked-key violations and `extends` cycles exit 2
- [x] `qgate run` with no inputs writes a valid report and exits 0
- [x] `go vet`, `staticcheck` and `go test -race` are green in CI

## Verification (2026-10-16, at 15ce3f4)

Run against a fresh clone of `HEAD` and the binary built from it.

- `qgate validate --stage staging` on the reference config (with `QGATE_LOCAL_LLM_URL` set) exits 0 and prints the resolved config. An invalid config exits 2 with `.qgate.yaml: profiles.staging.enforcement: value must be one of 'advisory', 'blocking'`.
- A repo config changing a locked key exits 2 (`.qgate.yaml: scoring.agent_max_penalty: locked by the org config; it must stay 10`). An `extends` cycle exits 2 (`profiles.a.extends: extends cycle: a → b → a`).
- `qgate run` in an empty directory exits 0, prints `Gate outcome: PASS` and writes `qgate-report.json`. Schema validity of the report is asserted by the Seam A tests.
- Every step of `.github/workflows/ci.yml` passes on the clean clone: gofmt, `go vet`, staticcheck 2025.1.1, `go test -race`, `CGO_ENABLED=0` build. Tests also pass under Go 1.25.0, the version `go.mod` declares and CI installs, and the binary cross-builds for linux/amd64.

**CI:** the first run on `github.com/assimoes/qgate` (run 38922188088, commit 5bcceff, 2026-10-16) passed `scripts/check.sh`, which runs gofmt, vet and staticcheck through golangci-lint, and `go test -race`.
