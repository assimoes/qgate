package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/assimoes/qgate/internal/clitest"
	"github.com/assimoes/qgate/schemas"
)

func TestRunWithNoInputsWritesValidReportAndPasses(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{}, "run")

	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	validateReport(t, res.Report)

	report := res.DecodeReport(t)
	gate := report["gate"].(map[string]any)
	if got := gate["outcome"]; got != "pass" {
		t.Errorf("gate.outcome = %v, want pass", got)
	}
	if got := report["run"].(map[string]any)["stage"]; got != "dev" {
		t.Errorf("run.stage = %v, want dev (the default stage)", got)
	}
	if got := report["findings"].([]any); len(got) != 0 {
		t.Errorf("findings = %v, want none", got)
	}
	if got := report["scores"].(map[string]any)["overall"]; got != nil {
		t.Errorf("scores.overall = %v, want null when no category is assessed", got)
	}
	if !strings.Contains(strings.ToLower(res.Stdout), "gate outcome: pass") {
		t.Errorf("stdout does not lead with the gate outcome:\n%s", res.Stdout)
	}
}

func TestRunRecordsTheSelectedStage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"default", nil, nil, "dev"},
		{"flag", nil, []string{"--stage", "staging"}, "staging"},
		{"environment", map[string]string{"QGATE_STAGE": "prod"}, nil, "prod"},
		{"flag beats environment", map[string]string{"QGATE_STAGE": "prod"}, []string{"--stage", "staging"}, "staging"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := clitest.Run(t, clitest.Options{Env: tt.env}, append([]string{"run"}, tt.args...)...)
			if res.Code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
			}
			if got := res.DecodeReport(t)["run"].(map[string]any)["stage"]; got != tt.want {
				t.Errorf("run.stage = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestRunWithMissingExplicitConfigIsAConfigError(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{Env: map[string]string{"QGATE_CONFIG": "does-not-exist.yaml"}}, "run")

	if res.Code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "does-not-exist.yaml") {
		t.Errorf("stderr does not name the missing config:\n%s", res.Stderr)
	}
}

func TestRunWarnsWhenOnlyBuiltinPolicyIsActive(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{}, "run")

	if !strings.Contains(res.Stderr, "only builtin policy is active") {
		t.Errorf("stderr missing builtin-policy warning:\n%s", res.Stderr)
	}
}

func validateReport(t *testing.T, report []byte) {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.Report))
	if err != nil {
		t.Fatalf("parse report schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("report.schema.json", doc); err != nil {
		t.Fatalf("add report schema: %v", err)
	}
	sch, err := c.Compile("report.schema.json")
	if err != nil {
		t.Fatalf("compile report schema: %v", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(report))
	if err != nil {
		t.Fatalf("report is not JSON: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("report does not match schema: %v", err)
	}
}

func TestRunDoesNotWarnWhenRepoConfigExists(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".qgate.yaml", ".qgate.yml"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("version: 1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			res := clitest.Run(t, clitest.Options{Dir: dir}, "run")
			if strings.Contains(res.Stderr, "only builtin policy is active") {
				t.Errorf("unexpected builtin-policy warning with %s present:\n%s", name, res.Stderr)
			}
		})
	}
}

func TestRunExitsThreeWhenReportCannotBeWritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	res := clitest.Run(t, clitest.Options{Dir: dir}, "run")
	if res.Code != 3 {
		t.Fatalf("exit code = %d, want 3; stderr:\n%s", res.Code, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "write json report") {
		t.Errorf("stderr does not explain the failure:\n%s", res.Stderr)
	}
}

func TestRunAppliesTheResolvedProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".qgate.yaml"),
		[]byte("version: 1\nprofiles:\n  staging: { enforcement: blocking, scope: files }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{Dir: dir}, "run", "--stage", "staging")
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	validateReport(t, res.Report)
	run := res.DecodeReport(t)["run"].(map[string]any)
	if run["enforcement"] != "blocking" || run["scope"] != "files" {
		t.Errorf("run = %v, want blocking enforcement and files scope", run)
	}
	resolved := res.DecodeReport(t)["resolved_config"].(map[string]any)
	if resolved["stage"] != "staging" {
		t.Errorf("resolved_config.stage = %v, want staging", resolved["stage"])
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".qgate.yaml"), []byte("version: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := clitest.Run(t, clitest.Options{Dir: dir}, "run")
	if res.Code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.Code, res.Stderr)
	}
	if res.Report != nil {
		t.Error("a report was written for an invalid config")
	}
}
