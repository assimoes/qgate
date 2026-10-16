# 01: Walking skeleton

**What to build:** A buildable `qgate` binary whose `run` command, with no config and no inputs, walks the whole pipeline end to end: it writes a schema-valid JSON report, prints the gate outcome on the console and exits 0. Establishes the in-process CLI entry point (args, env, working directory, output streams in; exit code out) and the Seam A test harness every later ticket uses, plus `qgate version` and CI for the qgate repository itself.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `qgate run` with no inputs writes a JSON report that validates against the report schema and exits 0
- [x] `qgate run` prints a warning that only builtin policy is active when no repo config exists
- [x] `qgate version` prints version, commit, build date and scoring algorithm version
- [x] Exit codes 0/1/2/3 are defined in one place and an unknown command or flag exits 2
- [x] A Seam A test helper invokes the CLI in-process and captures exit code, stdout, stderr and the written report
- [x] CI runs `go vet`, `staticcheck` and `go test -race` and is green

## Verification

CI on `github.com/assimoes/qgate` has run the workflow since 2026-10-16: the first run (38922188088, commit 5bcceff) passed `scripts/check.sh` (gofmt, vet and staticcheck through golangci-lint, `go test -race`) and the static build, and every push since has been green.
