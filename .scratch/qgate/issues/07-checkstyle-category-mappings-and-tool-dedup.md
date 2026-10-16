# 07: Checkstyle, category mappings and tool dedup

**What to build:** Teams can feed Checkstyle-format analyzers, map their vocabulary onto qgate's, and never see one issue counted twice.

**Blocked by:** 05 (First findings: git context, SARIF and gating scope)

**Status:** ready-for-agent

- [ ] Checkstyle XML is ingested (Seam C golden tests over golangci-lint and ESLint output)
- [x] Per-input `severity_map` applies before builtin defaults
- [x] `category_mappings` apply in order, first match wins; else `category_default`; else `quality`
- [x] Findings from different inputs with identical fingerprints or identical `(path, start_line, CWE set)` merge: highest severity survives, ties go to the alphabetically first input name, the rest are listed as merged sources
- [x] Dedup output is identical regardless of input order

## Open

- The Seam C goldens run against hand-written fixtures in golangci-lint's and ESLint's checkstyle shape, not captured output (neither linter was installed). Replace `internal/ingest/checkstyle/testdata/{golangci,eslint}.xml` with real output and run `go test ./internal/ingest/checkstyle -update`, then tick the first box.

## Demo

Build the binary once from the repo root: `go build -o /tmp/qgate ./cmd/qgate`. Then paste the script below into a terminal (it needs `git` and `jq`). It sets up a repo where a change touches a function with a SQL injection, plus reports from three analyzers: golangci-lint in Checkstyle format, and Semgrep and CodeQL in SARIF, both flagging the same injection.

```bash
# 1. A repo where a change adds a line next to a SQL injection
rm -rf /tmp/qgate-demo-07 && mkdir /tmp/qgate-demo-07 && cd /tmp/qgate-demo-07
git init -q -b main && git config user.email demo@example.com && git config user.name Demo
printf 'package app\n\nfunc Find(id string) {\n\tdb.Query("SELECT * FROM users WHERE id=" + id)\n}\n' > app.go
git add app.go && git commit -qm base && BASE=$(git rev-parse HEAD)
printf 'package app\n\nfunc Find(id string) {\n\tdb.Query("SELECT * FROM users WHERE id=" + id)\n\tlog.Printf("looked up %%s", id)\n}\n' > app.go
git commit -qam "add logging"

# Three analyzers' reports: golangci-lint (checkstyle), Semgrep and CodeQL (SARIF).
# Semgrep and CodeQL both flag the SQL injection on line 4 (CWE-89).
mkdir reports
cat > reports/golangci.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<checkstyle version="5.0">
  <file name="app.go">
    <error line="5" column="2" severity="error" message="Error return value is not checked" source="errcheck"></error>
    <error line="5" column="2" severity="warning" message="cyclomatic complexity is high" source="gocyclo"></error>
  </file>
</checkstyle>
EOF
cat > reports/semgrep.sarif <<'EOF'
{"version": "2.1.0", "runs": [{"tool": {"driver": {"name": "Semgrep", "rules": [{"id": "sql-injection", "properties": {"tags": ["CWE-89: SQL Injection"]}}]}},
  "results": [{"ruleId": "sql-injection", "level": "error", "message": {"text": "SQL built from user input"},
    "locations": [{"physicalLocation": {"artifactLocation": {"uri": "app.go"}, "region": {"startLine": 4}}}]}]}]}
EOF
cat > reports/codeql.sarif <<'EOF'
{"version": "2.1.0", "runs": [{"tool": {"driver": {"name": "CodeQL", "rules": [{"id": "go/sql-injection", "properties": {"security-severity": "9.8", "tags": ["external/cwe/cwe-089"]}}]}},
  "results": [{"ruleId": "go/sql-injection", "message": {"text": "Query depends on a user-provided value"},
    "locations": [{"physicalLocation": {"artifactLocation": {"uri": "app.go"}, "region": {"startLine": 4}}}]}]}]}
EOF
cat > .qgate.yaml <<'EOF'
version: 1
inputs:
  - { name: golangci, format: checkstyle, path: reports/golangci.xml, severity_map: { warning: low } }
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif }
  - { name: codeql, format: sarif, path: reports/codeql.sarif }
category_mappings:
  - { match: { cwe: "CWE-*" }, category: security }
  - { match: { rule_id: "golangci:gocyclo" }, category: maintainability }
profiles:
  dev: { scope: files }
EOF

# 2. Run the gate
echo "== 2"; /tmp/qgate run --base $BASE; echo "exit=$?"
# 3. Three findings, not four
echo "== 3"; jq -c '.findings[] | {rule_id, severity, category}' qgate-report.json
# 4. Semgrep's duplicate is folded into CodeQL's critical finding
echo "== 4"; jq -c '.findings[] | select(.rule_id == "codeql:go/sql-injection") | .merged_sources[] | {input, rule_id, severity}' qgate-report.json
# 5. Input order does not matter
echo "== 5"; cp qgate-report.json first.json
cat > reordered.yaml <<'EOF'
version: 1
inputs:
  - { name: codeql, format: sarif, path: reports/codeql.sarif }
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif }
  - { name: golangci, format: checkstyle, path: reports/golangci.xml, severity_map: { warning: low } }
category_mappings:
  - { match: { cwe: "CWE-*" }, category: security }
  - { match: { rule_id: "golangci:gocyclo" }, category: maintainability }
profiles:
  dev: { scope: files }
EOF
/tmp/qgate run --base $BASE --config reordered.yaml > /dev/null && diff <(jq -S .findings first.json) <(jq -S .findings qgate-report.json) && echo "findings identical"
# 6. A broken CWE pattern is caught before anything runs
echo "== 6"; sed 's/"CWE-\*"/"CWE-["/' .qgate.yaml > broken.yaml; /tmp/qgate validate --config broken.yaml; echo "exit=$?"
```

What to point out:

1. **Step 2:** one gate run reads both formats. Security scores 60 because of one critical finding; maintainability scores 98 because `severity_map` turned gocyclo's `warning` into `low`.
2. **Step 3:** three findings, not four. Each lands in the right category: the CWE mapping sends the injection to security, the rule mapping sends gocyclo to maintainability, and errcheck falls back to quality.
3. **Step 4:** Semgrep's report of the same injection is not lost. It is listed as a merged source of CodeQL's finding, which survived because it is more severe.
4. **Step 5:** listing the inputs in a different order produces identical findings.
5. **Step 6:** a broken CWE pattern is rejected with exit 2, naming the file, the path and the pattern.
