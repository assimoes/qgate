package cli_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"qgate/internal/clitest"
)

// checkstyleXML builds a Checkstyle report; paths are written as given.
func checkstyleXML(files map[string][][3]string) string { // path → [line, severity, source]
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<checkstyle version="5.0">` + "\n")
	for _, path := range sortedTools(toResults(files)) {
		fmt.Fprintf(&b, "  <file name=%q>\n", path)
		for _, e := range files[path] {
			fmt.Fprintf(&b, "    <error line=%q column=\"1\" severity=%q message=\"msg from %s\" source=%q></error>\n",
				e[0], e[1], e[2], e[2])
		}
		b.WriteString("  </file>\n")
	}
	b.WriteString("</checkstyle>\n")
	return b.String()
}

func toResults(files map[string][][3]string) map[string][]sarifResult {
	out := map[string][]sarifResult{}
	for k := range files {
		out[k] = nil
	}
	return out
}

func TestCheckstyleInput(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - name: golangci
    format: checkstyle
    path: reports/golangci.xml
    category_default: maintainability
    severity_map: { warning: low }
`)
	repo.Write("reports/golangci.xml", checkstyleXML(map[string][][3]string{
		"a.go":               {{"5", "error", "errcheck"}, {"5", "warning", "gocyclo"}, {"5", "info", "godot"}, {"5", "fatal", "typecheck"}},
		repo.Dir + "/new.go": {{"1", "error", "staticcheck"}}, // ESLint writes absolute paths
	}))
	found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))

	want := map[string]string{
		"golangci:errcheck":    "high",   // builtin error → high
		"golangci:gocyclo":     "low",    // severity_map beats the builtin warning → medium
		"golangci:godot":       "low",    // builtin info → low
		"golangci:typecheck":   "medium", // unmapped label
		"golangci:staticcheck": "high",
	}
	for rule, sev := range want {
		f, ok := found[rule]
		if !ok {
			t.Errorf("%s missing; found %v", rule, keys(found))
			continue
		}
		if f["severity"] != sev || f["category"] != "maintainability" || f["status"] != "new" {
			t.Errorf("%s = severity %v, category %v, status %v; want %s, maintainability, new", rule, f["severity"], f["category"], f["status"], sev)
		}
	}
	if p := found["golangci:staticcheck"]["location"].(map[string]any)["path"]; p != "new.go" {
		t.Errorf("absolute path not made repo-relative: %v", p)
	}
}

func TestCategoryMappings(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: semgrep, format: sarif, path: s.sarif, category_default: testing }
  - { name: lint, format: sarif, path: l.sarif }
category_mappings:
  - { match: { cwe: "*" }, category: security }
  - { match: { rule_id: "semgrep:perf-*" }, category: maintainability }
  - { match: { rule_id: "semgrep:*", path: "new.go" }, category: dependencies }
  - { match: { rule_id: "semgrep:perf-*" }, category: quality }
`)
	repo.Write("s.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "perf-loop-with-cwe", level: "error", path: "a.go", line: 5, cwe: "CWE-400"}, // cwe wins: first match
		{rule: "perf-loop", level: "error", path: "a.go", line: 5},                          // second mapping; the fourth never applies
		{rule: "anything", level: "error", path: "new.go", line: 1},                         // rule and path must both match
		{rule: "anything-else", level: "error", path: "a.go", line: 5},                      // no mapping: category_default
	}}))
	repo.Write("l.sarif", sarifLog(map[string][]sarifResult{"Lint": {
		{rule: "style", level: "error", path: "new.go", line: 1}, // no mapping, no default: quality
	}}))
	found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
	for rule, cat := range map[string]string{
		"semgrep:perf-loop-with-cwe": "security",
		"semgrep:perf-loop":          "maintainability",
		"semgrep:anything":           "dependencies",
		"semgrep:anything-else":      "testing",
		"lint:style":                 "quality",
	} {
		if got := found[rule]["category"]; got != cat {
			t.Errorf("%s category = %v, want %s", rule, got, cat)
		}
	}
}

const dedupConfig = `version: 1
inputs:
%s`

func dedupRepo(t *testing.T, inputOrder []string) (*clitest.Repo, string) {
	t.Helper()
	repo, base, _ := history(t)
	lines := map[string]string{
		"semgrep": "  - { name: semgrep, format: sarif, path: semgrep.sarif, category_default: security }\n",
		"codeql":  "  - { name: codeql, format: sarif, path: codeql.sarif, category_default: security }\n",
		"bandit":  "  - { name: bandit, format: sarif, path: bandit.sarif, category_default: security }\n",
	}
	var inputs strings.Builder
	for _, name := range inputOrder {
		inputs.WriteString(lines[name])
	}
	repo.Write(".qgate.yaml", fmt.Sprintf(dedupConfig, inputs.String()))
	repo.Write("semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "sqli", level: "error", path: "a.go", line: 5, cwe: "CWE-89"},  // high
		{rule: "xss", level: "warning", path: "a.go", line: 5, cwe: "CWE-79"}, // medium; ties with bandit
		{rule: "lonely", level: "error", path: "a.go", line: 5},               // no CWE: never merged
	}}))
	repo.Write("codeql.sarif", sarifLog(map[string][]sarifResult{"CodeQL": {
		{rule: "go/sql-injection", secSev: "9.8", path: "a.go", line: 5, cwe: "CWE-89"}, // critical
	}}))
	repo.Write("bandit.sarif", sarifLog(map[string][]sarifResult{"Bandit": {
		{rule: "B703", level: "warning", path: "a.go", line: 5, cwe: "CWE-79"}, // medium
		{rule: "B999", level: "error", path: "a.go", line: 5},                  // no CWE: never merged
	}}))
	return repo, base
}

func TestToolFindingsAreDeduplicated(t *testing.T) {
	t.Parallel()
	repo, base := dedupRepo(t, []string{"semgrep", "codeql", "bandit"})
	found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))

	if len(found) != 4 {
		t.Fatalf("findings = %v, want 4 (two merged pairs and two unrelated)", keys(found))
	}
	// SQL injection: CodeQL's critical survives; Semgrep's high is merged.
	sqli, ok := found["codeql:go/sql-injection"]
	if !ok {
		t.Fatalf("the critical CodeQL finding did not survive: %v", keys(found))
	}
	merged := sqli["merged_sources"].([]any)
	if len(merged) != 1 {
		t.Fatalf("merged_sources = %v", merged)
	}
	m := merged[0].(map[string]any)
	if m["input"] != "semgrep" || m["rule_id"] != "semgrep:sqli" || m["severity"] != "high" || m["fingerprint"] == "" {
		t.Errorf("merged source = %v", m)
	}
	// XSS: equal severity, so the alphabetically first input (bandit) wins.
	if _, ok := found["bandit:B703"]; !ok {
		t.Errorf("tie should go to bandit: %v", keys(found))
	}
	if _, ok := found["semgrep:xss"]; ok {
		t.Error("semgrep:xss should have been merged into bandit:B703")
	}
	for _, unrelated := range []string{"semgrep:lonely", "bandit:B999"} {
		if _, ok := found[unrelated]; !ok {
			t.Errorf("%s (no CWE) should not be merged", unrelated)
		}
	}
}

func TestDedupIsIndependentOfInputOrder(t *testing.T) {
	t.Parallel()
	findingsJSON := func(order []string) string {
		repo, base := dedupRepo(t, order)
		res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
		if res.Code != 0 {
			t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
		}
		out, _ := json.Marshal(res.DecodeReport(t)["findings"])
		return string(out)
	}
	want := findingsJSON([]string{"semgrep", "codeql", "bandit"})
	for _, order := range [][]string{{"bandit", "codeql", "semgrep"}, {"codeql", "semgrep", "bandit"}} {
		if got := findingsJSON(order); got != want {
			t.Errorf("findings differ for input order %v:\n%s\nvs\n%s", order, got, want)
		}
	}
}
