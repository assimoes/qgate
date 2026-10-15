package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qgate/internal/clitest"
)

// gateRepo is a history with one new high finding in security and one in
// quality, plus an out-of-scope critical one.
func gateRepo(t *testing.T, profile string) (*clitest.Repo, string) {
	t.Helper()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif, category_default: security }
  - { name: lint, format: sarif, path: reports/lint.sarif }
profiles:
  dev:
`+profile)
	repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "sqli", level: "error", path: "a.go", line: 5},
		{rule: "old", level: "error", secSev: "9.5", path: "a.go", line: 2},
	}}))
	repo.Write("reports/lint.sarif", sarifLog(map[string][]sarifResult{"Lint": {
		{rule: "style", level: "error", path: "new.go", line: 1},
	}}))
	return repo, base
}

func gateSection(t *testing.T, res clitest.Result) map[string]any {
	t.Helper()
	return res.DecodeReport(t)["gate"].(map[string]any)
}

func TestScoresInTheReport(t *testing.T) {
	t.Parallel()
	repo, base := gateRepo(t, "    enforcement: advisory\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if res.Code != 0 {
		t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
	}
	validateReport(t, res.Report)
	scores := res.DecodeReport(t)["scores"].(map[string]any)
	cats := scores["categories"].(map[string]any)
	// security: 10000 − 2000 (one new high; the critical is out of scope) = 80
	// quality:  10000 − 2000 = 80
	if cats["security"] != float64(80) || cats["quality"] != float64(80) {
		t.Errorf("categories = %v, want security 80 and quality 80", cats)
	}
	// No input declares maintainability, testing or dependencies.
	for _, c := range []string{"maintainability", "testing", "dependencies"} {
		if v, ok := cats[c]; !ok || v != nil {
			t.Errorf("%s = %v (present %v), want null", c, v, ok)
		}
	}
	if scores["overall"] != float64(80) {
		t.Errorf("overall = %v, want 80", scores["overall"])
	}
	if !strings.Contains(res.Stdout, "security: 80") {
		t.Errorf("console does not list category scores:\n%s", res.Stdout)
	}
}

func TestEnforcement(t *testing.T) {
	t.Parallel()
	const violated = "    gate: { max_new: { high: 0 }, category_min: { security: 90 } }\n"
	tests := []struct {
		name    string
		profile string
		code    int
		outcome string
	}{
		{"blocking with a violation exits 1", "    enforcement: blocking\n" + violated, 1, "fail"},
		{"advisory with a violation warns and exits 0", "    enforcement: advisory\n" + violated, 0, "warn"},
		{"blocking and clean passes", "    enforcement: blocking\n    gate: { max_new: { critical: 0 } }\n", 0, "pass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base := gateRepo(t, tt.profile)
			res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
			if res.Code != tt.code {
				t.Fatalf("exit code = %d, want %d; stderr:\n%s", res.Code, tt.code, res.Stderr)
			}
			validateReport(t, res.Report)
			g := gateSection(t, res)
			if g["outcome"] != tt.outcome {
				t.Errorf("gate.outcome = %v, want %s", g["outcome"], tt.outcome)
			}
			if !strings.HasPrefix(strings.ToLower(res.Stdout), "gate outcome: "+tt.outcome) {
				t.Errorf("console does not lead with the outcome:\n%s", res.Stdout)
			}
		})
	}
}

func TestFailedGateChecksAreExplained(t *testing.T) {
	t.Parallel()
	repo, base := gateRepo(t, "    enforcement: blocking\n    gate: { category_min: { security: 90 } }\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	checks := gateSection(t, res)["checks"].([]any)
	c := checks[0].(map[string]any)
	if c["check"] != "category_min.security" || c["expected"] != ">= 90" || c["actual"] != float64(80) || c["result"] != "fail" {
		t.Errorf("check = %v", c)
	}
	if !strings.Contains(res.Stdout, "category_min.security") || !strings.Contains(res.Stdout, ">= 90") {
		t.Errorf("console does not explain the failed check:\n%s", res.Stdout)
	}
}

func TestFailedInputDegradesTheRun(t *testing.T) {
	t.Parallel()
	repo, base := gateRepo(t, "    enforcement: blocking\n")
	if err := os.Remove(filepath.Join(repo.Dir, "reports", "lint.sarif")); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	g := gateSection(t, res)
	if g["outcome"] != "warn" || g["degraded"] != true {
		t.Errorf("gate = %v, want a degraded warn", g)
	}
	// The failed input assessed nothing, so quality is not assessed.
	if cats := res.DecodeReport(t)["scores"].(map[string]any)["categories"].(map[string]any); cats["quality"] != nil {
		t.Errorf("quality = %v, want null when its only input failed", cats["quality"])
	}
}

func TestScoreReproducesASavedReport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		profile string
		code    int
	}{
		{"failing blocking gate", "    enforcement: blocking\n    gate: { max_new: { high: 0 }, overall_min: 50 }\n", 1},
		{"passing gate", "    enforcement: blocking\n    gate: { overall_min: 50 }\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base := gateRepo(t, tt.profile)
			run := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
			if run.Code != tt.code {
				t.Fatalf("run exit code = %d, want %d; stderr:\n%s", run.Code, tt.code, run.Stderr)
			}
			// Score from a directory with no repo, no config and no inputs.
			elsewhere := t.TempDir()
			saved := filepath.Join(elsewhere, "saved.json")
			if err := os.WriteFile(saved, run.Report, 0o600); err != nil {
				t.Fatal(err)
			}
			res := clitest.Run(t, clitest.Options{Dir: elsewhere}, "score", "--report", "saved.json")
			if res.Code != tt.code {
				t.Fatalf("score exit code = %d, want %d; stderr:\n%s", res.Code, tt.code, res.Stderr)
			}
			if !strings.Contains(res.Stdout, "reproduced") {
				t.Errorf("stdout does not confirm reproduction:\n%s", res.Stdout)
			}
		})
	}
}

func TestScoreDetectsATamperedReport(t *testing.T) {
	t.Parallel()
	repo, base := gateRepo(t, "    enforcement: blocking\n    gate: { max_new: { high: 0 } }\n")
	run := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)

	var report map[string]any
	if err := json.Unmarshal(run.Report, &report); err != nil {
		t.Fatal(err)
	}
	for _, f := range report["findings"].([]any) {
		f.(map[string]any)["severity"] = "low" // hide the high findings
	}
	tampered, _ := json.Marshal(report)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.json"), tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{Dir: dir}, "score", "--report", "r.json")
	if res.Code != 3 || !strings.Contains(res.Stderr, "do not match") {
		t.Errorf("exit code = %d, want 3 reporting a mismatch; stderr:\n%s", res.Code, res.Stderr)
	}
}

func TestScoreReportErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{"score"}, 2},                             // --report is required
		{[]string{"score", "--report", "missing.json"}, 2}, // no such file
		{[]string{"score", "--report", "bad.json"}, 2},     // not a report
	} {
		res := clitest.Run(t, clitest.Options{Dir: dir}, tt.args...)
		if res.Code != tt.code {
			t.Errorf("%v: exit code = %d, want %d; stderr:\n%s", tt.args, res.Code, tt.code, res.Stderr)
		}
	}
}
