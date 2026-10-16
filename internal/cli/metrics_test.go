package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qgate/internal/clitest"
)

// a.go has 10 lines; history changes line 5. The coverage reports below
// instrument lines 2..6 of a.go: line 5 (changed) is not hit, the rest are.
// Repo: 4 of 5 lines covered = 80%. Diff: 0 of 1 changed line = 0%.

func coberturaXML(filename string) string {
	return `<?xml version="1.0" ?>
<coverage line-rate="0.8" branch-rate="0.5" version="1.9" timestamp="1">
  <sources><source>/build/src</source></sources>
  <packages><package name="app"><classes><class name="a" filename="` + filename + `">
    <lines>
      <line number="2" hits="3"/>
      <line number="3" hits="1" branch="true" condition-coverage="50% (1/2)"/>
      <line number="4" hits="1"/>
      <line number="5" hits="0" branch="true" condition-coverage="0% (0/2)"/>
      <line number="6" hits="2"/>
    </lines>
  </class></classes></package></packages>
</coverage>
`
}

const lcovInfo = `TN:
SF:a.go
DA:2,3
DA:3,1
DA:4,1
DA:5,0
DA:6,2
BRDA:3,0,0,1
BRDA:3,0,1,-
BRDA:5,0,0,0
BRDA:5,0,1,-
LF:5
LH:4
end_of_record
`

const junitXML = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="pay" tests="3">
    <testcase name="TestCharge" classname="pay"/>
    <testcase name="TestRefund" classname="pay"><failure message="want 1">boom</failure></testcase>
    <testcase name="TestSlow" classname="pay"><skipped/></testcase>
  </testsuite>
  <testsuite name="db" tests="2">
    <testsuite name="nested">
      <testcase name="TestConn" classname="db"><error message="panic"/></testcase>
    </testsuite>
    <testcase name="TestQuery" classname="db"/>
  </testsuite>
</testsuites>
`

// metricsByKey indexes a report's metrics as "name@scope".
func metricsByKey(t *testing.T, res clitest.Result) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	for _, m := range res.DecodeReport(t)["metrics"].([]any) {
		mm := m.(map[string]any)
		out[mm["name"].(string)+"@"+mm["scope"].(string)] = mm["value"].(float64)
	}
	return out
}

func TestCoverageMetrics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, format, file, content string
	}{
		{"cobertura with a repo-relative path", "cobertura", "coverage.xml", coberturaXML("a.go")},
		{"cobertura with a Go import path", "cobertura", "coverage.xml", coberturaXML("example.com/org/app/a.go")},
		{"lcov", "lcov", "lcov.info", lcovInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write(".qgate.yaml", fmt.Sprintf("version: 1\ninputs:\n  - { name: cov, format: %s, path: %s }\n", tt.format, tt.file))
			repo.Write(tt.file, tt.content)
			res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
			if res.Code != 0 {
				t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
			}
			validateReport(t, res.Report)
			got := metricsByKey(t, res)
			want := map[string]float64{
				"coverage.line@repo":   80,
				"coverage.line@diff":   0,
				"coverage.branch@repo": 25, // 1 of 4 branches
				"coverage.branch@diff": 0,  // 0 of line 5's 2
			}
			for k, v := range want {
				if g, ok := got[k]; !ok || g != v {
					t.Errorf("%s = %v (present %v), want %v", k, g, ok, v)
				}
			}
			// A coverage input assesses no category on its own.
			cats := res.DecodeReport(t)["scores"].(map[string]any)["categories"].(map[string]any)
			if cats["quality"] != nil {
				t.Errorf("quality = %v, want null: coverage input declared no category", cats["quality"])
			}
		})
	}
}

func TestDiffCoverageIsVacuouslyFullWhenNoInstrumentedLineChanged(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: cov, format: lcov, path: lcov.info }\n")
	// Only same.go (unchanged) is instrumented.
	repo.Write("lcov.info", "SF:same.go\nDA:1,0\nDA:2,1\nend_of_record\n")
	got := metricsByKey(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
	if got["coverage.line@diff"] != 100 || got["coverage.line@repo"] != 50 {
		t.Errorf("metrics = %v, want diff 100 (vacuous) and repo 50", got)
	}
	if _, ok := got["coverage.branch@repo"]; ok {
		t.Error("coverage.branch reported although the report has no branch data")
	}
}

func TestJUnitMetrics(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: tests, format: junit, path: junit.xml }\n")
	repo.Write("junit.xml", junitXML)
	got := metricsByKey(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
	want := map[string]float64{"tests.total@repo": 5, "tests.failed@repo": 2, "tests.skipped@repo": 1}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
}

func TestMetricGateChecks(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: cov, format: lcov, path: lcov.info }
  - { name: tests, format: junit, path: junit.xml }
profiles:
  dev:
    enforcement: blocking
    gate:
      metrics:
        - { name: coverage.line, scope: repo, op: ">=", value: 80 }
        - { name: coverage.line, scope: diff, op: ">", value: 0 }
        - { name: tests.failed, op: "==", value: 0 }
        - { name: tests.skipped, op: "<=", value: 1 }
        - { name: tests.total, op: "<", value: 6 }
        - { name: tests.total, op: "!=", value: 5 }
        - { name: mutation.score, op: ">=", value: 50 }
        - { name: mutation.score, op: ">=", value: 50, optional: true, scope: diff }
`)
	repo.Write("lcov.info", lcovInfo)
	repo.Write("junit.xml", junitXML)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if res.Code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.Code, res.Stderr)
	}
	checks := map[string]map[string]any{}
	for _, c := range gateSection(t, res)["checks"].([]any) {
		cm := c.(map[string]any)
		checks[cm["check"].(string)] = cm
	}
	want := []struct{ check, expected, result string }{
		{"metrics.coverage.line@repo", ">= 80", "pass"},
		{"metrics.coverage.line@diff", "> 0", "fail"},
		{"metrics.tests.failed@repo", "== 0", "fail"},
		{"metrics.tests.skipped@repo", "<= 1", "pass"},
		{"metrics.tests.total@repo", "< 6", "pass"},
		{"metrics.tests.total@repo#2", "!= 5", "fail"},
		{"metrics.mutation.score@repo", ">= 50", "fail"},
		{"metrics.mutation.score@diff", ">= 50", "skipped"},
	}
	for _, w := range want {
		c, ok := checks[w.check]
		if !ok {
			t.Errorf("gate check %s missing; have %v", w.check, mapKeys(checks))
			continue
		}
		if c["expected"] != w.expected || c["result"] != w.result {
			t.Errorf("%s = expected %v, result %v; want %s, %s", w.check, c["expected"], c["result"], w.expected, w.result)
		}
	}
	if note, _ := checks["metrics.mutation.score@repo"]["note"].(string); !strings.Contains(note, "not reported") {
		t.Errorf("missing metric note = %q", note)
	}

	// The saved report re-gates to the same result.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.json"), res.Report, 0o600); err != nil {
		t.Fatal(err)
	}
	if again := clitest.Run(t, clitest.Options{Dir: dir}, "score", "--report", "r.json"); again.Code != 1 {
		t.Errorf("score --report exit code = %d, want 1; stderr:\n%s", again.Code, again.Stderr)
	}
}

func mapKeys(m map[string]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
