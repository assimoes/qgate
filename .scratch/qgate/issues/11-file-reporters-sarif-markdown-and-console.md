# 11: File reporters: SARIF, Markdown and console

**What to build:** Every run leaves human- and machine-readable artifacts beyond the JSON report.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** resolved

- [x] Console shows outcome first, then category scores, failed gate checks and top findings; color only on a TTY without `NO_COLOR`
- [x] SARIF output contains new findings only by default, one run per source
- [x] Markdown output matches the PR summary content and is capped at 60,000 characters with a pointer to artifacts
- [x] `--output format=path` and `reporting.outputs` both work
- [x] A reporter failure does not change the exit code, except failing to write the JSON report (exit 3)
- [x] Golden tests cover console (no color), Markdown and SARIF output

## Demo

Build the binary once from the repo root: `go build -o /tmp/qgate ./cmd/qgate`. Then paste the script below (it needs `git` and `jq`). A change has a critical SQL injection from Semgrep and an unused variable from a Checkstyle linter; the config asks for JSON, Markdown and SARIF artifacts.

```bash
# 1. A change with findings from two analyzers, and a blocking gate
rm -rf /tmp/qgate-demo-11 && mkdir /tmp/qgate-demo-11 && cd /tmp/qgate-demo-11
git init -q -b main && git config user.email demo@example.com && git config user.name Demo
printf 'package app\n' > app.go && git add . && git commit -qm base && BASE=$(git rev-parse HEAD)
printf 'package app\n\nvar q = "SELECT " + id\nvar x = 1\n' > app.go && git commit -qam change
cat > semgrep.sarif <<'EOF'
{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"Semgrep","rules":[{"id":"sql-concat","shortDescription":{"text":"SQL built by concatenation"},"properties":{"security-severity":"9.1","tags":["CWE-89"]}}]}},"results":[
 {"ruleId":"sql-concat","message":{"text":"Use a parameterized query"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"app.go"},"region":{"startLine":3}}}]}]}]}
EOF
cat > lint.xml <<'EOF'
<checkstyle><file name="app.go"><error line="4" severity="warning" message="x is unused" source="unused"/></file></checkstyle>
EOF
cat > .qgate.yaml <<'EOF'
version: 1
inputs:
  - { name: semgrep, format: sarif, path: semgrep.sarif, category_default: security }
  - { name: lint, format: checkstyle, path: lint.xml }
profiles:
  dev: { enforcement: blocking, gate: { max_new: { critical: 0 } } }
reporting:
  outputs:
    - { format: json, path: out/qgate-report.json }
    - { format: markdown, path: out/summary.md }
    - { format: sarif, path: out/qgate.sarif }
EOF

# 2. The console: outcome first, scores, why it failed, the top findings
echo "== 2"; /tmp/qgate run --base $BASE; echo "exit=$?"
# 3. Every configured artifact is there
echo "== 3"; ls out
# 4. The Markdown summary, ready to post on a PR
echo "== 4"; cat out/summary.md
# 5. SARIF for code scanning: one run per analyzer, new findings, ranked
echo "== 5"; jq -c '.runs[] | {tool: .tool.driver.name, results: [.results[] | {ruleId, level}]}' out/qgate.sarif
# 6. --output replaces the configured list; the JSON report is always written
echo "== 6"; rm -rf out; /tmp/qgate run --base $BASE --output markdown=pr.md > /dev/null; echo "exit=$?"; ls out pr.md qgate-report.json 2>&1
# 7. A broken output path is a warning, not a failure: the exit code is the gate's
echo "== 7"; /tmp/qgate run --base $BASE --output markdown=app.go/x.md > /dev/null; echo "exit=$?"
```

What to point out:

1. **Step 2:** the console leads with the outcome, then scores, the failed gate check with expected and actual values, and the findings most severe first. In a terminal the outcome and severities are colored; `NO_COLOR` turns that off.
2. **Step 3:** every configured artifact is written, in directories qgate creates.
3. **Step 4:** the Markdown summary is what a PR comment will carry: outcome, scores, why the gate failed, counts by severity and the top findings, pointing to the full JSON report.
4. **Step 5:** the SARIF has one run per analyzer and only the new findings, ready for GitHub code scanning.
5. **Step 6:** `--output` replaces the configured list for one run; the canonical JSON report is still written.
6. **Step 7:** an output that cannot be written is a warning; the exit code is still the gate's.
