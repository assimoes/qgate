# 08: Metrics: coverage and test results

**What to build:** Coverage and test-result reports become metrics the gate can check.

**Blocked by:** 06 (Scores, gate checks, enforcement and score --report)

**Status:** ready-for-human

- [x] Cobertura and LCOV produce `coverage.line` and `coverage.branch` at `repo` metric scope and at `diff` metric scope computed from changed ranges
- [x] JUnit produces `tests.total`, `tests.failed`, `tests.skipped`
- [x] `metrics[]` gate checks support all six operators and fail on a missing metric unless `optional: true`
- [ ] Seam C golden tests cover real Cobertura, LCOV and JUnit output

## Open

- The Seam C goldens run against hand-written fixtures in each tool's output shape. Replace `internal/ingest/{cobertura,lcov,junit}/testdata/*` with captured output and run `go test ./internal/ingest/... -update`, then tick the last box.

## Demo

Build the binary once from the repo root: `go build -o /tmp/qgate ./cmd/qgate`. Then paste the script below (it needs `git` and `jq`). It sets up a change that adds a coupon branch to `cart.js` without a test that exercises it, with an LCOV report and JUnit results from the CI test step.

```bash
# 1. A repo where a change adds an untested branch to cart.js
rm -rf /tmp/qgate-demo-08 && mkdir /tmp/qgate-demo-08 && cd /tmp/qgate-demo-08
git init -q -b main && git config user.email demo@example.com && git config user.name Demo
printf 'function total(items) {\n  let sum = 0\n  for (const i of items) sum += i.price\n  return sum\n}\n' > cart.js
git add cart.js && git commit -qm base && BASE=$(git rev-parse HEAD)
printf 'function total(items, coupon) {\n  let sum = 0\n  for (const i of items) sum += i.price\n  if (coupon) sum -= coupon.amount\n  return sum\n}\n' > cart.js
git commit -qam "support coupons"

# Coverage from the test run: line 4 (the new coupon branch) never ran.
mkdir reports
printf 'SF:cart.js\nDA:1,3\nDA:2,3\nDA:3,9\nDA:4,0\nDA:5,3\nBRDA:4,0,0,0\nBRDA:4,0,1,3\nend_of_record\n' > reports/lcov.info
cat > reports/junit.xml <<'EOF'
<testsuites><testsuite name="cart">
  <testcase name="totals items"/>
  <testcase name="empty cart"/>
  <testcase name="coupon applies"><skipped/></testcase>
</testsuite></testsuites>
EOF
cat > .qgate.yaml <<'EOF'
version: 1
inputs:
  - { name: coverage, format: lcov, path: reports/lcov.info }
  - { name: tests, format: junit, path: reports/junit.xml }
profiles:
  dev:
    enforcement: blocking
    gate:
      metrics:
        - { name: coverage.line, scope: diff, op: ">=", value: 80 }
        - { name: tests.failed, op: "==", value: 0 }
        - { name: tests.skipped, op: "==", value: 0 }
        - { name: coverage.branch, scope: repo, op: ">=", value: 50, optional: true }
EOF

# 2. The gate fails: the changed line is uncovered and a test is skipped
echo "== 2"; /tmp/qgate run --base $BASE; echo "exit=$?"
# 3. The metrics the gate saw
echo "== 3"; jq -c '.metrics[] | {name, scope, value, input: .source.name}' qgate-report.json
# 4. Every metric check with its expected and actual value
echo "== 4"; jq -c '.gate.checks[] | {check, expected, actual, result}' qgate-report.json
# 5. The saved report re-gates to the same answer without running anything
echo "== 5"; cp qgate-report.json saved.json; /tmp/qgate score --report saved.json 2>&1 | tail -2; echo "exit=${PIPESTATUS[0]}"
# 6. Un-skip the test and cover the coupon line: the gate passes
echo "== 6"; printf 'SF:cart.js\nDA:1,4\nDA:2,4\nDA:3,9\nDA:4,1\nDA:5,4\nBRDA:4,0,0,1\nBRDA:4,0,1,3\nend_of_record\n' > reports/lcov.info
sed -i.bak 's|<testcase name="coupon applies"><skipped/></testcase>|<testcase name="coupon applies"/>|' reports/junit.xml
/tmp/qgate run --base $BASE; echo "exit=$?"
```

What to point out:

1. **Step 2:** the blocking gate fails on two metric checks, and the console names both with expected and actual values. Diff coverage is 50%: of the two lines the change touched, the signature ran and the coupon line did not.
2. **Step 3:** coverage and test counts at the scopes the gate can check, each tagged with the input that reported it. Repo coverage is 80%; the change's own lines are what fail.
3. **Step 4:** every metric check, including the passing ones. The optional branch check passes; had the metric been missing it would have been skipped, not failed.
4. **Step 5:** the saved report re-gates to the same failure (exit 1) with no git, coverage or test run.
5. **Step 6:** once the coupon line is covered and the test un-skipped, the same gate passes.
