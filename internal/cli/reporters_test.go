package cli_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/assimoes/qgate/internal/clitest"
)

var update = flag.Bool("update", false, "rewrite golden files")

// pinnedNow keeps reporter output deterministic.
var pinnedNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// reporterRepo has findings of every severity, one out of scope, from two
// inputs, and a blocking gate that fails.
func reporterRepo(t *testing.T, outputs string) (*clitest.Repo, string) {
	t.Helper()
	repo, base, _ := history(t)
	repo.Write("semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "sqli", level: "error", secSev: "9.8", path: "a.go", line: 5, cwe: "CWE-89"},
		{rule: "weak-hash", level: "warning", path: "a.go", line: 5},
		{rule: "todo", level: "note", path: "new.go", line: 1},
		{rule: "old", level: "error", path: "a.go", line: 2}, // out of scope
	}}))
	repo.Write("lint.sarif", sarifLog(map[string][]sarifResult{"Lint": {
		{rule: "unused", level: "error", path: "new.go", line: 2},
	}}))
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: semgrep, format: sarif, path: semgrep.sarif, category_default: security }
  - { name: lint, format: sarif, path: lint.sarif }
profiles:
  dev: { enforcement: blocking, gate: { max_new: { critical: 0 }, category_min: { security: 90 } } }
`+outputs)
	return repo, base
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "reporters", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from its golden (run with -update after checking the change):\n%s", name, got)
	}
}

func TestReporterGoldens(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, `reporting:
  outputs:
    - { format: json, path: qgate-report.json }
    - { format: sarif, path: out/qgate.sarif }
    - { format: markdown, path: out/qgate-report.md }
`)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Now: pinnedNow}, "run", "--base", base)
	if res.Code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", res.Code, res.Stderr)
	}
	golden(t, "console.txt", []byte(res.Stdout))
	for _, name := range []string{"qgate.sarif", "qgate-report.md"} {
		data, err := os.ReadFile(filepath.Join(repo.Dir, "out", name))
		if err != nil {
			t.Fatalf("%s was not written: %v", name, err)
		}
		golden(t, name, data)
	}
}

func TestConsoleLeadsWithTheOutcomeAndListsTopFindings(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Now: pinnedNow}, "run", "--base", base)
	lines := strings.Split(res.Stdout, "\n")
	if lines[0] != "Gate outcome: FAIL" {
		t.Errorf("first line = %q", lines[0])
	}
	critical := strings.Index(res.Stdout, "semgrep:sqli")
	low := strings.Index(res.Stdout, "semgrep:todo")
	if critical < 0 || low < 0 || critical > low {
		t.Errorf("top findings missing or not most severe first:\n%s", res.Stdout)
	}
	if strings.Contains(res.Stdout, "semgrep:old") {
		t.Error("out-of-scope finding listed among top findings")
	}
	if strings.Contains(res.Stdout, "\x1b[") {
		t.Error("color codes in non-terminal output")
	}
}

func TestConsoleColor(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "")
	tests := []struct {
		name  string
		env   map[string]string
		color bool
	}{
		{"terminal", nil, true},
		{"terminal with NO_COLOR", map[string]string{"NO_COLOR": "1"}, false},
	}
	for _, tt := range tests {
		res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Env: tt.env, Terminal: true}, "run", "--base", base)
		if got := strings.Contains(res.Stdout, "\x1b["); got != tt.color {
			t.Errorf("%s: color = %v, want %v", tt.name, got, tt.color)
		}
	}
}

func TestSARIFOutput(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--output", "sarif=qgate.sarif")
	data, err := os.ReadFile(filepath.Join(repo.Dir, "qgate.sarif"))
	if err != nil {
		t.Fatalf("not written: %v; stderr:\n%s", err, res.Stderr)
	}
	validateSARIF(t, data)
	var log struct {
		Runs []struct {
			Tool struct {
				Driver struct{ Name string } `json:"driver"`
			} `json:"tool"`
			Results []struct {
				RuleID  string `json:"ruleId"`
				Level   string `json:"level"`
				Message struct {
					Text string `json:"text"`
				} `json:"message"`
			} `json:"results"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	runs := map[string][]string{}
	for _, r := range log.Runs {
		for _, res := range r.Results {
			runs[r.Tool.Driver.Name] = append(runs[r.Tool.Driver.Name], res.RuleID+"="+res.Level)
			if res.Message.Text == "" {
				t.Errorf("%s has no message text, which SARIF requires (§3.11)", res.RuleID)
			}
		}
	}
	if len(log.Runs) != 2 {
		t.Errorf("runs = %d, want one per input", len(log.Runs))
	}
	if got := strings.Join(runs["semgrep"], " "); got != "semgrep:sqli=error semgrep:todo=note semgrep:weak-hash=warning" {
		t.Errorf("semgrep results = %s, want the three new findings only", got)
	}
}

func validateSARIF(t *testing.T, data []byte) {
	t.Helper()
	schemaFile, err := os.Open(filepath.Join("testdata", "sarif-schema-2.1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer schemaFile.Close()
	doc, err := jsonschema.UnmarshalJSON(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("sarif.json", doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("sarif.json")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("SARIF output is not JSON: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Errorf("SARIF output does not match the SARIF 2.1.0 schema: %v", err)
	}
}

func TestOutputFlagReplacesConfiguredOutputs(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "reporting:\n  outputs:\n    - { format: markdown, path: configured.md }\n")
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--output", "markdown=flag/summary.md")
	if _, err := os.Stat(filepath.Join(repo.Dir, "flag", "summary.md")); err != nil {
		t.Errorf("flag output missing: %v; stderr:\n%s", err, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(repo.Dir, "configured.md")); err == nil {
		t.Error("configured output written although --output replaced it")
	}
	if res.Report == nil {
		t.Error("the canonical JSON report must always be written")
	}
}

func TestOutputFlagErrors(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"markdown", "pdf=out.pdf", "=x"} {
		res := clitest.Run(t, clitest.Options{}, "run", "--output", arg)
		if res.Code != 2 {
			t.Errorf("--output %s: exit code = %d, want 2; stderr:\n%s", arg, res.Code, res.Stderr)
		}
	}
}

func TestReporterFailureDoesNotChangeTheExitCode(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "")
	// a.go is a file, so nothing can be written beneath it.
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--output", "markdown=a.go/summary.md", "--output", "json=qgate-report.json")
	if res.Code != 1 {
		t.Errorf("exit code = %d, want 1 (the gate's), not a reporter failure; stderr:\n%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "summary.md") {
		t.Errorf("stderr does not mention the failed reporter:\n%s", res.Stderr)
	}
}

func TestMarkdownIsCapped(t *testing.T) {
	t.Parallel()
	repo, base := reporterRepo(t, "suppressions:\n")
	// Hundreds of long warnings push the summary past its cap.
	var sups strings.Builder
	for i := range 300 {
		fmt.Fprintf(&sups, "  - { rule: \"semgrep:rule-%03d\", reason: \"%s\", expires: 2000-01-31 }\n", i, strings.Repeat("an old waiver ", 20))
	}
	cfg, err := os.ReadFile(filepath.Join(repo.Dir, ".qgate.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	repo.Write(".qgate.yaml", string(cfg)+sups.String())
	clitest.Run(t, clitest.Options{Dir: repo.Dir, Now: pinnedNow}, "run", "--base", base, "--output", "markdown=summary.md")
	data, err := os.ReadFile(filepath.Join(repo.Dir, "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(string(data))); n > 60000 {
		t.Errorf("markdown is %d characters, want at most 60000", n)
	}
	if !strings.Contains(string(data), "truncated") || !strings.Contains(string(data), "qgate-report.json") {
		t.Error("truncated markdown does not point to the full report")
	}
}
