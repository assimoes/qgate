package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assimoes/qgate/internal/clitest"
	"github.com/assimoes/qgate/internal/ingest"
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

func TestCoveragePathsMatchTheRightChangedFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		changePk bool    // also change pkg/a.go
		want     float64 // coverage.line@diff
	}{
		// Only a.go changed (line 2, covered). pkg/a.go's uncovered line 2
		// must not count: it is a different, unchanged file.
		{"unchanged file with the same base name", false, 100},
		// Both changed on line 2: a.go covered, pkg/a.go not.
		{"both files changed", true, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := clitest.NewRepo(t)
			repo.Write("a.go", clitest.Lines(3))
			repo.Write("pkg/a.go", clitest.Lines(3))
			base := repo.Commit("base")
			repo.Write("a.go", strings.Replace(clitest.Lines(3), "line 2\n", "line 2 changed\n", 1))
			if tt.changePk {
				repo.Write("pkg/a.go", strings.Replace(clitest.Lines(3), "line 2\n", "line 2 changed\n", 1))
			}
			repo.Commit("head")
			repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: cov, format: lcov, path: lcov.info }\n")
			repo.Write("lcov.info", "SF:a.go\nDA:2,1\nend_of_record\nSF:pkg/a.go\nDA:2,0\nend_of_record\n")
			got := metricsByKey(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
			if got["coverage.line@diff"] != tt.want {
				t.Errorf("coverage.line@diff = %v, want %v", got["coverage.line@diff"], tt.want)
			}
		})
	}
}

func TestCoberturaFilenamesAreRelativeToSources(t *testing.T) {
	t.Parallel()
	repo := clitest.NewRepo(t)
	repo.Write("src/app/views.py", clitest.Lines(3))
	base := repo.Commit("base")
	repo.Write("src/app/views.py", strings.Replace(clitest.Lines(3), "line 2\n", "line 2 changed\n", 1))
	repo.Commit("head")
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: cov, format: cobertura, path: coverage.xml }
profiles:
  dev: { enforcement: blocking, gate: { metrics: [{ name: coverage.line, scope: diff, op: ">=", value: 80 }] } }
`)
	repo.Write("coverage.xml", `<coverage><sources><source>`+repo.Dir+`/src</source></sources>
<packages><package><classes><class filename="app/views.py"><lines>
<line number="1" hits="1"/><line number="2" hits="0"/>
</lines></class></classes></package></packages></coverage>`)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if got := metricsByKey(t, res)["coverage.line@diff"]; got != 0 {
		t.Errorf("coverage.line@diff = %v, want 0 (src/app/views.py's changed line is uncovered)", got)
	}
	if res.Code != 1 {
		t.Errorf("exit code = %d, want 1: the gate must fail", res.Code)
	}
}

func TestRescoringDoesNotDependOnTheBinarysFormats(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: cov, format: lcov, path: lcov.info }
  - { name: lint, format: sarif, path: lint.sarif }
`)
	repo.Write("lcov.info", lcovInfo)
	repo.Write("lint.sarif", sarifLog(map[string][]sarifResult{"L": {{rule: "r", level: "error", path: "a.go", line: 5}}}))
	run := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if run.Code != 0 {
		t.Fatalf("exit code = %d; stderr:\n%s", run.Code, run.Stderr)
	}
	for _, in := range run.DecodeReport(t)["inputs"].([]any) {
		m := in.(map[string]any)
		want := map[string]string{"cov": "metrics", "lint": "findings"}[m["name"].(string)]
		if m["kind"] != want {
			t.Errorf("input %v kind = %v, want %s", m["name"], m["kind"], want)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.json"), run.Report, 0o600); err != nil {
		t.Fatal(err)
	}
	// A build that knows no formats at all still reproduces the decision.
	res := clitest.Run(t, clitest.Options{Dir: dir, Ingestors: ingest.NewRegistry()}, "score", "--report", "r.json")
	if res.Code != 0 {
		t.Errorf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
}
