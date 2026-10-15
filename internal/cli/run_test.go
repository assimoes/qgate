package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"qgate/internal/clitest"
	"qgate/schemas"
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
	res := clitest.Run(t, clitest.Options{}, "run", "--stage", "staging")

	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	if got := res.DecodeReport(t)["run"].(map[string]any)["stage"]; got != "staging" {
		t.Errorf("run.stage = %v, want staging", got)
	}
}

func TestRunStageFallsBackToEnvironment(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{Env: map[string]string{"QGATE_STAGE": "prod"}}, "run")

	if got := res.DecodeReport(t)["run"].(map[string]any)["stage"]; got != "prod" {
		t.Errorf("run.stage = %v, want prod from QGATE_STAGE", got)
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
