# 09: Input failures and run mode

**What to build:** Inputs can run commands directly, and a broken analyzer is handled according to policy instead of silently passing or wrongly failing the gate.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** resolved

- [x] `mode: run` executes an argv (no shell) in the repo root with a timeout and only the `env` allowlist, then parses its output
- [x] A missing file, failing command, timeout or parse error marks the input `failed` in the report
- [x] A `required` input failure, or any failure under `on_input_error: fail`, writes reports then exits 3
- [x] Otherwise the run is degraded, the outcome is at most `warn` for that reason, and categories only that input fed become not assessed
- [x] Infrastructure failures never exit 1

## Demo

Build the binary once from the repo root: `go build -o /tmp/qgate ./cmd/qgate`. Then paste the script below (it needs `git` and `jq`). Two stand-in analyzers are commands qgate runs itself: `lint.sh` finds a double charge and, like golangci-lint, exits 1 when it does; `slow.sh` hangs like a stuck dependency analyzer.

```bash
# 1. A repo with a change, and a stand-in "analyzer" script that, like
#    golangci-lint, exits 1 when it finds something
rm -rf /tmp/qgate-demo-09 && mkdir /tmp/qgate-demo-09 && cd /tmp/qgate-demo-09
git init -q -b main && git config user.email demo@example.com && git config user.name Demo
printf 'package app\n\nfunc Pay() {}\n' > app.go && git add app.go && git commit -qm base && BASE=$(git rev-parse HEAD)
printf 'package app\n\nfunc Pay() { charge(); charge() }\n' > app.go && git commit -qam "charge twice"
cat > lint.sh <<'EOF'
#!/bin/sh
cat <<SARIF
{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"lint","rules":[{"id":"double-charge"}]}},"results":[
 {"ruleId":"double-charge","level":"error","message":{"text":"charge() called twice (token seen: ${LINT_TOKEN:-none}, secret seen: ${AWS_SECRET:-none})"},
  "locations":[{"physicalLocation":{"artifactLocation":{"uri":"app.go"},"region":{"startLine":3}}}]}]}]}
SARIF
exit 1
EOF
printf '#!/bin/sh\nsleep 30\n' > slow.sh && chmod +x lint.sh slow.sh
cat > .qgate.yaml <<'EOF'
version: 1
inputs:
  - { name: lint, mode: run, format: sarif, command: ["./lint.sh"], env: [LINT_TOKEN], category_default: quality }
  - { name: deps, mode: run, format: sarif, command: ["./slow.sh"], timeout: 2s, category_default: dependencies }
profiles:
  dev: { enforcement: blocking, gate: { max_new: { critical: 0 } } }
EOF
export LINT_TOKEN=tok-123 AWS_SECRET=hunter2

# 2. qgate runs both commands itself: lint's exit 1 still yields its finding,
#    the slow analyzer times out, and the run is degraded but not failed
echo "== 2"; /tmp/qgate run --base $BASE; echo "exit=$?"
# 3. What each input did, and what the lint command could see
echo "== 3"; jq -c '.inputs[] | {name, status, warnings, error}' qgate-report.json
jq -r '.findings[0].message' qgate-report.json
# 4. Mark the slow analyzer required: the same run now exits 3, after writing the report
echo "== 4"; sed -i.bak 's/timeout: 2s,/timeout: 2s, required: true,/' .qgate.yaml
/tmp/qgate run --base $BASE > /dev/null; echo "exit=$?"; jq -c '.gate | {outcome, degraded}' qgate-report.json
# 5. Re-gating the saved report reaches the same exit code
echo "== 5"; cp qgate-report.json saved.json; /tmp/qgate score --report saved.json > /dev/null; echo "exit=$?"
```

What to point out:

1. **Step 2:** qgate runs both commands itself. Lint's exit status 1 does not lose its finding; the hung analyzer is killed after its 2-second timeout. The run is degraded, so the blocking gate warns instead of passing silently, and exits 0 because the failed input is optional.
2. **Step 3:** each input's outcome is in the report: lint ok with a warning about its exit status, deps failed with the reason. The lint command saw `LINT_TOKEN`, which its `env` allowlist names, but not `AWS_SECRET` from the same shell.
3. **Step 4:** once the analyzer is `required`, the same failure is fatal: exit 3, an infrastructure failure, never 1. The report is still written, so CI can upload it.
4. **Step 5:** re-gating the saved report reaches the same exit 3 without running anything.
