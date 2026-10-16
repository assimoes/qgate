package cli_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/assimoes/qgate/internal/clitest"
)

// script writes the executable shell script scan.sh into the repo.
func script(t *testing.T, repo *clitest.Repo, body string) {
	t.Helper()
	repo.Write("scan.sh", "#!/bin/sh\n"+body)
	if err := os.Chmod(filepath.Join(repo.Dir, "scan.sh"), 0o755); err != nil {
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
		{name: "arguments are passed without a shell", script: "test \"$1\" = '$HOME *' && cat finding.sarif\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh", "$HOME *"] }`, status: "ok", finding: true},
		{name: "non-zero exit with output is parsed, with a warning", script: "cat finding.sarif\nexit 1\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "ok", finding: true, message: "exited with status 1"},
		{name: "non-zero exit without output fails", script: "echo boom >&2\nexit 2\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "boom"},
		{name: "unparseable output fails", script: "echo not sarif\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "parse"},
		{name: "timeout fails", script: "sleep 10\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"], timeout: 200ms }`, status: "failed", message: "timed out after 200ms"},
		{name: "a command killed by a signal fails", script: "cat finding.sarif\nkill -9 $$\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "killed by signal"},
		{name: "a stale path file fails", script: "echo crashed >&2\nexit 2\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"], path: finding.sarif }`, status: "failed", message: "not written by"},
		{name: "the real end of stderr is quoted", script: "head -c 40000 /dev/zero | tr '\\0' x >&2\necho LAST LINE >&2\nexit 2\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "LAST LINE"},
		{name: "a parse failure quotes stderr", script: "echo 'warn: bad config' >&2\necho garbage\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "bad config"},
		{name: "output over the cap fails", script: "head -c 270000000 /dev/zero\n",
			input: `{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, status: "failed", message: "exceeds 256 MiB"},
		{name: "a command that cannot start fails", script: "",
			input: `{ name: scan, mode: run, format: sarif, command: ["./does-not-exist"] }`, status: "failed", message: "does-not-exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write("finding.sarif", oneFinding)
			script(t, repo, tt.script)
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
				t.Errorf("error/warnings = %.300s, want %q", text, tt.message)
			}
			if e, _ := in["error"].(string); len(e) > 3000 {
				t.Errorf("error is %d bytes; a command's stderr must be quoted as a short tail", len(e))
			}
		})
	}
}

func TestRunModeEnvironmentIsAnAllowlist(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	// The script reports what it can see as the finding's message.
	script(t, repo, `printf '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"S","rules":[{"id":"r"}]}},"results":[{"ruleId":"r","message":{"text":"allowed=%s secret=%s pwd=%s"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},"region":{"startLine":5}}}]}]}]}' "$ALLOWED" "$SECRET" "$(pwd)"
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
	if !strings.HasSuffix(strings.TrimSpace(msg), filepath.Base(repo.Dir)) {
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
		t.Error("quality = null, want assessed: lint still assessed it")
	}
	if cats["dependencies"] != nil {
		t.Errorf("dependencies = %v, want null: only the failed input assessed it", cats["dependencies"])
	}
}

// processGone reports whether the process whose pid is in file has exited.
func processGone(t *testing.T, file string) bool {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return true
	}
	return p.Signal(syscall.Signal(0)) != nil
}

func TestRunModeChildProcesses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, script, input, status string
	}{
		// The command finished with complete output; its lingering child
		// must neither fail the input nor outlive the run.
		{"a child outliving a finished command", "cat finding.sarif\n(sleep 30) &\necho $! > child.pid\nexit 0\n",
			`{ name: scan, mode: run, format: sarif, command: ["./scan.sh"] }`, "ok"},
		// A timed-out command's children are killed with it.
		{"a timed-out command's child", "(sleep 30) &\necho $! > child.pid\nsleep 30\n",
			`{ name: scan, mode: run, format: sarif, command: ["./scan.sh"], timeout: 2s }`, "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write("finding.sarif", oneFinding)
			script(t, repo, tt.script)
			repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - "+tt.input+"\n")
			start := time.Now()
			res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Env: map[string]string{"PATH": os.Getenv("PATH")}}, "run", "--base", base)
			if took := time.Since(start); took > 8*time.Second {
				t.Errorf("run took %s, want it to end without waiting for the child", took)
			}
			if in := inputStatus(t, res, "scan"); in["status"] != tt.status {
				t.Errorf("status = %v, want %s; input = %v", in["status"], tt.status, in)
			}
			if !processGone(t, filepath.Join(repo.Dir, "child.pid")) {
				t.Error("the command's child process is still running")
			}
		})
	}
}

func TestCommandStderrIsRedacted(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	script(t, repo, "echo \"auth failed token=$LINT_TOKEN\" >&2\nexit 2\n")
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: scan, mode: run, format: sarif, command: [\"./scan.sh\"], env: [LINT_TOKEN] }\n")
	env := map[string]string{"PATH": os.Getenv("PATH"), "LINT_TOKEN": "ghp_supersecret123"}
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Env: env}, "run", "--base", base)
	msg := fmt.Sprint(inputStatus(t, res, "scan")["error"])
	if strings.Contains(string(res.Report)+res.Stdout+res.Stderr, "ghp_supersecret123") {
		t.Errorf("the token reached the output; error = %s", msg)
	}
	if !strings.Contains(msg, "[REDACTED:LINT_TOKEN]") {
		t.Errorf("error = %s, want the token replaced by [REDACTED:LINT_TOKEN]", msg)
	}
}

// The run's deadline reaches the user through more than one path (the
// input step and the report writer both notice it), so this guards the
// outcome, exit 3 naming the deadline, rather than one code path.
func TestRunDeadlineIsNotReportedAsTheInputsTimeout(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	script(t, repo, "sleep 30\n")
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: scan, mode: run, format: sarif, command: [\"./scan.sh\"], timeout: 60s }\n")
	// The whole run's deadline expires long before the input's own timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Context: ctx, Env: map[string]string{"PATH": os.Getenv("PATH")}}, "run", "--base", base)
	if res.Code != 3 || !strings.Contains(res.Stderr, "deadline exceeded") || strings.Contains(res.Stderr, "timed out after 1m0s") {
		t.Errorf("exit code = %d, want 3 naming the run's deadline; stderr:\n%s", res.Code, res.Stderr)
	}
}
