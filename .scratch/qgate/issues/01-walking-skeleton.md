# 01: Walking skeleton

**What to build:** A buildable `qgate` binary whose `run` command, with no config and no inputs, walks the whole pipeline end to end: it writes a schema-valid JSON report, prints the gate outcome on the console and exits 0. Establishes the in-process CLI entry point (args, env, working directory, output streams in; exit code out) and the Seam A test harness every later ticket uses, plus `qgate version` and CI for the qgate repository itself.

**Blocked by:** None (can start immediately)

**Status:** resolved

- [ ] `qgate run` with no inputs writes a JSON report that validates against the report schema and exits 0
- [ ] `qgate run` prints a warning that only builtin policy is active when no repo config exists
- [ ] `qgate version` prints version, commit, build date and scoring algorithm version
- [ ] Exit codes 0/1/2/3 are defined in one place and an unknown command or flag exits 2
- [ ] A Seam A test helper invokes the CLI in-process and captures exit code, stdout, stderr and the written report
- [ ] CI runs `go vet`, `staticcheck` and `go test -race` and is green
