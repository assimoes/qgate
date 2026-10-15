# 04: Gate: Phase 0 acceptance

**What to build:** Verify Phase 0 is complete before any Phase 1 work starts. No new behavior; fix whatever the checks reveal.

**Blocked by:** 01 (Walking skeleton), 02 (Repo config and profiles), 03 (Org config and locked keys)

**Status:** ready-for-agent

- [ ] `qgate validate --stage staging` prints the resolved reference config; invalid configs error with file, path and message
- [ ] Locked-key violations and `extends` cycles exit 2
- [ ] `qgate run` with no inputs writes a valid report and exits 0
- [ ] `go vet`, `staticcheck` and `go test -race` are green in CI
