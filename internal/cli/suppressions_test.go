package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/assimoes/qgate/internal/clitest"
)

func suppressionRepo(t *testing.T, suppressions, profile string) (*clitest.Repo, string) {
	t.Helper()
	repo, base, _ := history(t)
	repo.Write("s.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "sqli", level: "error", path: "a.go", line: 5},
		{rule: "sqli", level: "error", path: "new.go", line: 1},
		{rule: "xss", level: "error", path: "a.go", line: 5},
		{rule: "xss", level: "error", path: "new.go", line: 1},
	}}))
	repo.Write(".qgate.yaml", fmt.Sprintf(`version: 1
inputs:
  - { name: semgrep, format: sarif, path: s.sarif, category_default: security }
suppressions:
%s
profiles:
  dev:
%s`, suppressions, profile))
	return repo, base
}

// statusByKey indexes findings as "rule@path".
func statusByKey(t *testing.T, res clitest.Result) map[string]map[string]any {
	t.Helper()
	if res.Code != 0 && res.Code != 1 {
		t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
	}
	out := map[string]map[string]any{}
	for _, f := range res.DecodeReport(t)["findings"].([]any) {
		m := f.(map[string]any)
		out[m["rule_id"].(string)+"@"+m["location"].(map[string]any)["path"].(string)] = m
	}
	return out
}

func TestSuppressionsMatchRuleAndPath(t *testing.T) {
	t.Parallel()
	repo, base := suppressionRepo(t, `  - { rule: "semgrep:sql*", paths: ["new.go"], reason: "generated query builder", expires: 2999-12-31 }`, "    enforcement: advisory\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	validateReport(t, res.Report)
	got := statusByKey(t, res)
	if f := got["semgrep:sqli@new.go"]; f["status"] != "suppressed" || f["status_note"] != "generated query builder" {
		t.Errorf("new.go sqli = %v / %v, want suppressed with the reason", f["status"], f["status_note"])
	}
	for _, k := range []string{"semgrep:sqli@a.go", "semgrep:xss@a.go", "semgrep:xss@new.go"} {
		if got[k]["status"] != "new" {
			t.Errorf("%s = %v, want new (path or rule does not match)", k, got[k]["status"])
		}
	}
}

func TestSuppressionWithoutPathsCoversEveryPath(t *testing.T) {
	t.Parallel()
	repo, base := suppressionRepo(t, `  - { rule: "semgrep:sqli", reason: "accepted risk" }`, "    enforcement: advisory\n")
	got := statusByKey(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
	if got["semgrep:sqli@a.go"]["status"] != "suppressed" || got["semgrep:sqli@new.go"]["status"] != "suppressed" {
		t.Errorf("sqli findings = %v, %v, want both suppressed", got["semgrep:sqli@a.go"]["status"], got["semgrep:sqli@new.go"]["status"])
	}
	if got["semgrep:xss@a.go"]["status"] != "new" {
		t.Error("xss must stay new")
	}
}

func TestExpiredSuppressionsAreIgnoredWithAWarning(t *testing.T) {
	t.Parallel()
	repo, base := suppressionRepo(t, `  - { rule: "semgrep:sqli", reason: "temporary", expires: 2000-01-31 }`, "    enforcement: advisory\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	validateReport(t, res.Report)
	if got := statusByKey(t, res); got["semgrep:sqli@a.go"]["status"] != "new" {
		t.Errorf("sqli = %v, want new: the suppression expired", got["semgrep:sqli@a.go"]["status"])
	}
	warnings := fmt.Sprint(res.DecodeReport(t)["warnings"])
	if !strings.Contains(warnings, "semgrep:sqli") || !strings.Contains(warnings, "2000-01-31") {
		t.Errorf("report warnings = %s, want the expired suppression named", warnings)
	}
	if !strings.Contains(res.Stdout, "expired") {
		t.Errorf("console does not show the warning:\n%s", res.Stdout)
	}
}

func TestSuppressedFindingsNeverScoreOrGate(t *testing.T) {
	t.Parallel()
	const gate = "    enforcement: blocking\n    gate: { max_new: { high: 0 } }\n"
	all := `  - { rule: "semgrep:*", reason: "migrating to a new scanner" }`
	repo, base := suppressionRepo(t, all, gate)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0: every high finding is suppressed; stderr:\n%s", res.Code, res.Stderr)
	}
	cats := res.DecodeReport(t)["scores"].(map[string]any)["categories"].(map[string]any)
	if cats["security"] != float64(100) {
		t.Errorf("security = %v, want 100", cats["security"])
	}
	// And the saved report re-gates identically.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.json"), res.Report, 0o600); err != nil {
		t.Fatal(err)
	}
	if again := clitest.Run(t, clitest.Options{Dir: dir}, "score", "--report", "r.json"); again.Code != 0 {
		t.Errorf("score --report exit code = %d; stderr:\n%s", again.Code, again.Stderr)
	}
}

func TestSuppressionExpiryMustBeARealDate(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{{
		name:   "February 30th",
		config: "version: 1\nsuppressions:\n  - { rule: \"x:*\", reason: r, expires: 2027-02-30 }\n",
		want:   []string{".qgate.yaml: suppressions[0].expires:", "not a valid date"},
	}})
}

func TestSuppressionIsValidThroughItsExpiryDay(t *testing.T) {
	t.Parallel()
	today := time.Now().UTC().Format("2006-01-02")
	repo, base := suppressionRepo(t, `  - { rule: "semgrep:sqli", reason: "until the end of today", expires: `+today+` }`, "    enforcement: advisory\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if got := statusByKey(t, res)["semgrep:sqli@a.go"]["status"]; got != "suppressed" {
		t.Errorf("status = %v, want suppressed on its expiry day", got)
	}
}
