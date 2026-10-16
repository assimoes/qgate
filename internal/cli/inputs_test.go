package cli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/assimoes/qgate/internal/clitest"
)

// script writes an executable shell script into the repo.
func script(t *testing.T, repo *clitest.Repo, name, body string) {
	t.Helper()
	repo.Write(name, "#!/bin/sh\n"+body)
	if err := os.Chmod(filepath.Join(repo.Dir, name), 0o755); err != nil {
		t.Fatal(err)
	}
}

// inputStatus returns one input's entry from the report.
func inputStatus(t *testing.T, res clitest.Result, name string) map[string]any {
	t.Helper()
	for _, in := range res.DecodeReport(t)["inputs"].([]any) {
		if m := in.(map[string]any); m["name"] == name {
			return m
		}
	}
	t.Fatalf("input %s missing from the report", name)
	return nil
}

var oneFinding = sarifLog(map[string][]sarifResult{"Scan": {{rule: "r", level: "warning", path: "a.go", line: 5}}})

func TestRunModeInputs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		script  string
		input   string
		status  string
		finding bool
		message string // in the error or warnings
	}{
		{name: "stdout is parsed", script: "cat finding.sarif\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "ok", finding: true},
		{name: "path is read after the command", script: "cp finding.sarif out.sarif\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"], path: out.sarif }`, status: "ok", finding: true},
		{name: "arguments are passed without a shell", script: "cat \"$1\"\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh", "finding.sarif"] }`, status: "ok", finding: true},
		{name: "non-zero exit with output is parsed, with a warning", script: "cat finding.sarif\nexit 1\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "ok", finding: true, message: "exited with status 1"},
		{name: "non-zero exit without output fails", script: "echo boom >&2\nexit 2\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "boom"},
		{name: "unparseable output fails", script: "echo not sarif\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "parse"},
		{name: "timeout fails", script: "sleep 10\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"], timeout: 200ms }`, status: "failed", message: "timed out after 200ms"},
		{name: "a command that cannot start fails", script: "",
			input: `{ name: scan, mode: run, format: sarif, command: ["./does-not-exist"] }`, status: "failed", message: "does-not-exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write("finding.sarif", oneFinding)
			script(t, repo, "scan.sh", tt.script)
			repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - "+tt.input+"\n")
			res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Env: map[string]string{"PATH": os.Getenv("PATH")}}, "run", "--base", base)
			if res.Code != 0 {
				t.Fatalf("exit code = %d, want 0 (not required); stderr:\n%s", res.Code, res.Stderr)
			}
			in := inputStatus(t, res, "scan")
			if in["status"] != tt.status {
				t.Errorf("status = %v, want %s; input = %v", in["status"], tt.status, in)
			}
			if got := len(res.DecodeReport(t)["findings"].([]any)) > 0; got != tt.finding {
				t.Errorf("finding present = %v, want %v", got, tt.finding)
			}
			if text := fmt.Sprint(in["error"], in["warnings"]); !strings.Contains(text, tt.message) {
				t.Errorf("error/warnings = %s, want %q", text, tt.message)
			}
		})
	}
}

func TestRunModeEnvironmentIsAnAllowlist(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	// The script reports what it can see as the finding's message.
	script(t, repo, "scan.sh", `printf '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"S","rules":[{"id":"r"}]}},"results":[{"ruleId":"r","message":{"text":"allowed=%s secret=%s pwd=%s"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},"region":{"startLine":5}}}]}]}]}' "$ALLOWED" "$SECRET" "$(pwd)"
`)
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: scan, mode: run, format: sarif, command: [\"./scan.sh\"], env: [ALLOWED] }\n")
	env := map[string]string{"PATH": os.Getenv("PATH"), "ALLOWED": "yes", "SECRET": "leak"}
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Env: env}, "run", "--base", base)
	fs := res.DecodeReport(t)["findings"].([]any)
	if len(fs) != 1 {
		t.Fatalf("findings = %v; stderr:\n%s", fs, res.Stderr)
	}
	msg := fs[0].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "allowed=yes") || !strings.Contains(msg, "secret= ") {
		t.Errorf("message = %q, want the allowlisted variable only", msg)
	}
	if !strings.Contains(msg, "pwd="+filepath.Base(repo.Dir)) && !strings.HasSuffix(strings.TrimSpace(msg), filepath.Base(repo.Dir)) {
		t.Errorf("message = %q, want the command to run in the repo root", msg)
	}
}

func TestInputFailurePolicy(t *testing.T) {
	t.Parallel()
	const healthy = "  - { name: lint, format: sarif, path: lint.sarif, category_default: quality }\n"
	tests := []struct {
		name    string
		broken  string // the failing input
		profile string
		code    int
		outcome string
	}{
		{"required input fails: exit 3", "  - { name: scan, format: sarif, path: missing.sarif, required: true }\n", "", 3, "warn"},
		{"on_input_error: fail: exit 3", "  - { name: scan, format: sarif, path: missing.sarif }\n",
			"  dev: { on_input_error: fail }\n", 3, "warn"},
		{"neither: degraded warn, exit 0", "  - { name: scan, format: sarif, path: missing.sarif }\n", "", 0, "warn"},
		{"required failure beats a failed blocking gate", "  - { name: scan, format: sarif, path: missing.sarif, required: true }\n",
			"  dev: { enforcement: blocking, gate: { max_new: { high: 0 } } }\n", 3, "fail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write("lint.sarif", sarifLog(map[string][]sarifResult{"L": {{rule: "r", level: "error", path: "a.go", line: 5}}}))
			cfg := "version: 1\ninputs:\n" + healthy + tt.broken
			if tt.profile != "" {
				cfg += "profiles:\n" + tt.profile
			}
			repo.Write(".qgate.yaml", cfg)
			res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
			if res.Code != tt.code {
				t.Fatalf("exit code = %d, want %d; stderr:\n%s", res.Code, tt.code, res.Stderr)
			}
			// Reports are written before exiting.
			validateReport(t, res.Report)
			if g := gateSection(t, res); g["outcome"] != tt.outcome || g["degraded"] != true {
				t.Errorf("gate = %v, want outcome %s, degraded", g, tt.outcome)
			}
			if !strings.Contains(res.Stdout, "Gate outcome") {
				t.Error("console report was not printed before exiting")
			}
			if tt.code == 3 && !strings.Contains(res.Stderr, "scan") {
				t.Errorf("stderr does not name the failed input:\n%s", res.Stderr)
			}

			// The saved report re-gates to the same exit code.
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "r.json"), res.Report, 0o600); err != nil {
				t.Fatal(err)
			}
			if again := clitest.Run(t, clitest.Options{Dir: dir}, "score", "--report", "r.json"); again.Code != tt.code {
				t.Errorf("score --report exit code = %d, want %d; stderr:\n%s", again.Code, tt.code, again.Stderr)
			}
		})
	}
}

func TestOnlyCategoriesFedSolelyByAFailedInputBecomeNotAssessed(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write("q.sarif", sarifLog(map[string][]sarifResult{"Q": {{rule: "r", level: "note", path: "a.go", line: 5}}}))
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: lint, format: sarif, path: q.sarif, category_default: quality }
  - { name: lint2, format: sarif, path: missing.sarif, category_default: quality }
  - { name: vuln, format: sarif, path: missing.sarif, category_default: dependencies }
`)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	cats := res.DecodeReport(t)["scores"].(map[string]any)["categories"].(map[string]any)
	if cats["quality"] == nil {
		t.Error("quality = null, want assessed: lint still covered it")
	}
	if cats["dependencies"] != nil {
		t.Errorf("dependencies = %v, want null: only the failed input covered it", cats["dependencies"])
	}
}
