package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"qgate/internal/clitest"
)

// referenceEnv is what the reference config needs to interpolate.
var referenceEnv = map[string]string{"QGATE_LOCAL_LLM_URL": "http://ollama.internal:11434/v1"}

func writeRepoConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func referenceConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "reference.qgate.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// dig walks a decoded YAML document by keys.
func dig(t *testing.T, doc any, keys ...string) any {
	t.Helper()
	cur := doc
	for i, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("%s is not a map", strings.Join(keys[:i], "."))
		}
		if cur, ok = m[k]; !ok {
			t.Fatalf("%s is missing", strings.Join(keys[:i+1], "."))
		}
	}
	return cur
}

func validateStage(t *testing.T, opts clitest.Options, args ...string) (clitest.Result, map[string]any) {
	t.Helper()
	res := clitest.Run(t, opts, append([]string{"validate"}, args...)...)
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(res.Stdout), &doc); err != nil {
		t.Fatalf("validate output is not YAML: %v\n%s", err, res.Stdout)
	}
	return res, doc
}

func TestValidatePrintsResolvedReferenceConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, ".qgate.yaml", referenceConfig(t))

	_, doc := validateStage(t, clitest.Options{Dir: dir, Env: referenceEnv}, "--stage", "staging")

	checks := []struct {
		path []string
		want any
	}{
		{[]string{"stage"}, "staging"},
		// staging overrides base
		{[]string{"profile", "enforcement"}, "blocking"},
		{[]string{"profile", "gate", "overall_min"}, 70},
		{[]string{"profile", "gate", "category_min", "security"}, 85},
		// inherited from base through extends
		{[]string{"profile", "scope"}, "diff"},
		{[]string{"profile", "on_agent_error"}, "warn"},
		// repo config over builtin defaults
		{[]string{"project", "name"}, "payments-api"},
		{[]string{"scoring", "agent_max_penalty"}, 15},
		// interpolated in a marked field
		{[]string{"providers", "local", "base_url"}, "http://ollama.internal:11434/v1"},
		// API keys stay as variable names
		{[]string{"providers", "anthropic", "api_key_env"}, "QGATE_ANTHROPIC_API_KEY"},
	}
	for _, c := range checks {
		if got := dig(t, doc, c.path...); got != c.want {
			t.Errorf("%s = %v, want %v", strings.Join(c.path, "."), got, c.want)
		}
	}
	if _, ok := doc["profiles"]; ok {
		t.Error("resolved config still lists all profiles; want only the selected one")
	}
}

func TestValidateWithoutRepoConfigUsesBuiltinDefaults(t *testing.T) {
	t.Parallel()
	_, doc := validateStage(t, clitest.Options{})

	if got := dig(t, doc, "stage"); got != "dev" {
		t.Errorf("stage = %v, want dev", got)
	}
	if got := dig(t, doc, "profile", "enforcement"); got != "advisory" {
		t.Errorf("profile.enforcement = %v, want advisory", got)
	}
	if got := dig(t, doc, "scoring", "severity_weights", "critical"); got != 40 {
		t.Errorf("scoring.severity_weights.critical = %v, want builtin 40", got)
	}
}

// configErrorCase is a repo config that must be rejected with exit 2 and an
// error naming the file, the dotted path and the problem.
type configErrorCase struct {
	name   string
	config string
	args   []string
	want   []string // substrings of stderr
}

func runConfigErrorCases(t *testing.T, cases []configErrorCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeRepoConfig(t, dir, ".qgate.yaml", tt.config)
			res := clitest.Run(t, clitest.Options{Dir: dir}, append([]string{"validate"}, tt.args...)...)
			if res.Code != 2 {
				t.Fatalf("exit code = %d, want 2; stdout:\n%s\nstderr:\n%s", res.Code, res.Stdout, res.Stderr)
			}
			for _, w := range tt.want {
				if !strings.Contains(res.Stderr, w) {
					t.Errorf("stderr missing %q:\n%s", w, res.Stderr)
				}
			}
		})
	}
}

func TestValidateRejectsSchemaViolations(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{
			name:   "bad enum deep in a profile",
			config: "version: 1\nprofiles:\n  staging:\n    enforcement: sometimes\n",
			want:   []string{".qgate.yaml: profiles.staging.enforcement:", "advisory", "blocking"},
		},
		{
			name:   "missing required field in a list element",
			config: "version: 1\ninputs:\n  - format: sarif\n",
			want:   []string{".qgate.yaml: inputs[0]:", "name"},
		},
		{
			name:   "unknown key (typo)",
			config: "version: 1\nscoring:\n  agent_max_penatly: 15\n",
			want:   []string{".qgate.yaml: scoring.agent_max_penatly: unknown property"},
		},
		{
			name:   "wrong type",
			config: "version: 1\nscoring:\n  agent_weight_pct: lots\n",
			want:   []string{".qgate.yaml: scoring.agent_weight_pct:", "integer"},
		},
		{
			name:   "missing version",
			config: "project:\n  name: x\n",
			want:   []string{".qgate.yaml:", "version"},
		},
		{
			name:   "invalid YAML",
			config: "version: 1\nproject: [unclosed\n",
			want:   []string{".qgate.yaml:", "invalid YAML"},
		},
		{
			name:   "API key written inline instead of by variable name",
			config: "version: 1\nproviders:\n  a:\n    type: anthropic\n    api_key: sk-123\n",
			want:   []string{".qgate.yaml: providers.a.api_key: unknown property"},
		},
		{
			name:   "unknown key in a profile",
			config: "version: 1\nprofiles:\n  dev:\n    enforcment: blocking\n",
			want:   []string{".qgate.yaml: profiles.dev.enforcment: unknown property"},
		},
		{
			name:   "every problem is reported at once",
			config: "version: 1\nscoring:\n  agent_weight_pct: lots\nprofiles:\n  dev:\n    scope: everything\n",
			want:   []string{"scoring.agent_weight_pct:", "profiles.dev.scope:"},
		},
	})
}

const agentsConfig = `version: 1
providers:
  local: { type: openai_compatible, base_url: "http://llm" }
agents:
  rev:
    kind: reviewer
    provider: local
    model: m
    prompt: { base: builtin://security-reviewer@1 }
`

func TestValidateRejectsSemanticErrors(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{
			name:   "unknown category in an input",
			config: "version: 1\ninputs:\n  - { name: x, format: sarif, path: a, category_default: secrity }\n",
			want:   []string{".qgate.yaml: inputs[0].category_default:", `unknown category "secrity"`},
		},
		{
			name:   "unknown category in a gate check",
			config: "version: 1\nprofiles:\n  dev:\n    gate: { category_min: { perf: 50 } }\n",
			want:   []string{".qgate.yaml: profiles.dev.gate.category_min.perf:", `unknown category "perf"`},
		},
		{
			name:   "unknown provider",
			config: strings.Replace(agentsConfig, "provider: local", "provider: anthropic", 1),
			want:   []string{"agents.rev.provider:", `unknown provider "anthropic"`},
		},
		{
			name:   "unknown agent in a profile",
			config: agentsConfig + "profiles:\n  dev: { agents: [rev, ghost] }\n",
			want:   []string{".qgate.yaml: profiles.dev.agents[1]:", `unknown agent "ghost"`},
		},
		{
			name:   "unknown agent in a profile that is not selected is still an error",
			config: agentsConfig + "profiles:\n  nightly: { agents: [ghost] }\n",
			want:   []string{".qgate.yaml: profiles.nightly.agents[0]:", `unknown agent "ghost"`},
		},
		{
			name:   "unknown category introduced by --set",
			config: "version: 1\n",
			args:   []string{"--set", "profile.gate.category_min.perf=50"},
			want:   []string{"--set: profile.gate.category_min.perf:", `unknown category "perf"`},
		},
		{
			name:   "unknown agent in a review rule",
			config: agentsConfig + "review_rules:\n  - { id: r, description: d, severity: high, category: security, agents: [ghost] }\n",
			want:   []string{"review_rules[0].agents[0]:", `unknown agent "ghost"`},
		},
		{
			name:   "duplicate input names",
			config: "version: 1\ninputs:\n  - { name: semgrep, format: sarif, path: a }\n  - { name: semgrep, format: sarif, path: b }\n",
			want:   []string{".qgate.yaml: inputs[1].name:", "duplicate name semgrep"},
		},
		{
			name:   "inputs without names are not reported as duplicates of each other",
			config: "version: 1\ninputs:\n  - { format: sarif, path: a }\n  - { format: sarif, path: b }\n",
			want:   []string{"inputs[0]: missing property 'name'"},
		},
		{
			name:   "invalid glob",
			config: "version: 1\nproject: { exclude: [\"vendor/[\"] }\n",
			want:   []string{"project.exclude[0]:", "invalid glob"},
		},
		{
			name:   "agent kind contradicts its builtin prompt",
			config: strings.Replace(agentsConfig, "kind: reviewer", "kind: triager", 1),
			want:   []string{"agents.rev.prompt.base:", "is for a reviewer", `"rev" is a triager`},
		},
		{
			name:   "unknown builtin prompt",
			config: strings.Replace(agentsConfig, "security-reviewer@1", "perf-reviewer@1", 1),
			want:   []string{"agents.rev.prompt.base:", `unknown builtin prompt "perf-reviewer"`},
		},
		{
			name:   "builtin prompt version that does not exist",
			config: strings.Replace(agentsConfig, "security-reviewer@1", "security-reviewer@9", 1),
			want:   []string{"agents.rev.prompt.base:", "no version 9"},
		},
	})
}

func TestValidateResolvesProfileInheritance(t *testing.T) {
	t.Parallel()
	const profiles = `version: 1
profiles:
  base:
    enforcement: advisory
    gate: { max_new: { critical: 0 }, overall_min: 50 }
  staging:
    extends: base
    enforcement: blocking
    gate: { max_new: { high: 0 } }
  prod:
    extends: staging
    gate: { overall_min: 80 }
`
	dir := t.TempDir()
	writeRepoConfig(t, dir, ".qgate.yaml", profiles)

	t.Run("three-level chain merges root first", func(t *testing.T) {
		t.Parallel()
		_, doc := validateStage(t, clitest.Options{Dir: dir}, "--stage", "prod")
		for _, c := range []struct {
			path []string
			want any
		}{
			{[]string{"profile", "enforcement"}, "blocking"},
			{[]string{"profile", "gate", "overall_min"}, 80},
			{[]string{"profile", "gate", "max_new", "critical"}, 0},
			{[]string{"profile", "gate", "max_new", "high"}, 0},
			{[]string{"profile", "scope"}, "diff"}, // profile default
		} {
			if got := dig(t, doc, c.path...); got != c.want {
				t.Errorf("%s = %v, want %v", strings.Join(c.path, "."), got, c.want)
			}
		}
		if _, ok := dig(t, doc, "profile").(map[string]any)["extends"]; ok {
			t.Error("resolved profile still has extends")
		}
	})
	t.Run("abstract profile can be selected", func(t *testing.T) {
		t.Parallel()
		_, doc := validateStage(t, clitest.Options{Dir: dir}, "--stage", "base")
		if got := dig(t, doc, "profile", "gate", "overall_min"); got != 50 {
			t.Errorf("profile.gate.overall_min = %v, want 50", got)
		}
	})
}

func TestValidateRejectsBrokenProfiles(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{
			name:   "extends cycle",
			config: "version: 1\nprofiles:\n  a: { extends: b }\n  b: { extends: a }\n",
			want:   []string{".qgate.yaml: profiles.", "extends cycle"},
		},
		{
			name:   "cycle in a profile that is not selected is still an error",
			config: "version: 1\nprofiles:\n  x: { extends: y }\n  y: { extends: x }\n",
			args:   []string{"--stage", "dev"},
			want:   []string{"extends cycle"},
		},
		{
			name:   "extends an unknown profile",
			config: "version: 1\nprofiles:\n  staging: { extends: bse }\n",
			want:   []string{".qgate.yaml: profiles.staging.extends:", `unknown profile "bse"`},
		},
		{
			name:   "unknown stage",
			config: "version: 1\n",
			args:   []string{"--stage", "qa"},
			want:   []string{`unknown stage "qa"`, "dev", "prod", "staging"},
		},
	})
}

func TestValidateAcceptsCustomCategoryDeclaredInWeights(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, ".qgate.yaml", `version: 1
scoring: { category_weights: { perf: 5 } }
inputs:
  - { name: bench, format: junit, path: a.xml, category_default: perf }
profiles:
  dev: { gate: { category_min: { perf: 60 } } }
`)
	_, doc := validateStage(t, clitest.Options{Dir: dir})
	if got := dig(t, doc, "profile", "gate", "category_min", "perf"); got != 60 {
		t.Errorf("profile.gate.category_min.perf = %v, want 60", got)
	}
}

func TestRepoConfigDiscovery(t *testing.T) {
	t.Parallel()
	named := func(name string) string { return "version: 1\nproject: { name: " + name + " }\n" }
	tests := []struct {
		name  string
		files map[string]string
		env   map[string]string
		args  []string
		want  string
	}{
		{"yaml before yml", map[string]string{".qgate.yaml": named("yaml"), ".qgate.yml": named("yml")}, nil, nil, "yaml"},
		{"yml when no yaml", map[string]string{".qgate.yml": named("yml")}, nil, nil, "yml"},
		{"QGATE_CONFIG beats discovery", map[string]string{".qgate.yaml": named("yaml"), "ci.yaml": named("env")},
			map[string]string{"QGATE_CONFIG": "ci.yaml"}, nil, "env"},
		{"--config beats QGATE_CONFIG", map[string]string{"ci.yaml": named("env"), "flag.yaml": named("flag")},
			map[string]string{"QGATE_CONFIG": "ci.yaml"}, []string{"--config", "flag.yaml"}, "flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tt.files {
				writeRepoConfig(t, dir, name, content)
			}
			_, doc := validateStage(t, clitest.Options{Dir: dir, Env: tt.env}, tt.args...)
			if got := dig(t, doc, "project", "name"); got != tt.want {
				t.Errorf("project.name = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestMissingExplicitConfigIsAConfigError(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{}, "validate", "--config", "nope.yaml")
	if res.Code != 2 || !strings.Contains(res.Stderr, "nope.yaml: repo config not found") {
		t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
	}
}

func TestSetOverridesApplyLast(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, ".qgate.yaml", "version: 1\nprofiles:\n  dev: { gate: { overall_min: 70 } }\n")

	_, doc := validateStage(t, clitest.Options{Dir: dir},
		"--set", "profile.gate.overall_min=80",
		"--set", "profile.enforcement=blocking",
		"--set", "llm.exclude_paths=[a/**, b/**]")

	if got := dig(t, doc, "profile", "gate", "overall_min"); got != 80 {
		t.Errorf("profile.gate.overall_min = %v, want 80 from --set", got)
	}
	if got := dig(t, doc, "profile", "enforcement"); got != "blocking" {
		t.Errorf("profile.enforcement = %v, want blocking", got)
	}
	if got := dig(t, doc, "llm", "exclude_paths").([]any); len(got) != 2 || got[0] != "a/**" {
		t.Errorf("llm.exclude_paths = %v, want [a/** b/**]", got)
	}
}

func TestSetRejectsInvalidOverrides(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{name: "not path=value", config: "version: 1\n", args: []string{"--set", "profile.gate"},
			want: []string{"--set:", "path=value"}},
		{name: "violates the schema", config: "version: 1\n", args: []string{"--set", "profile.enforcement=sometimes"},
			want: []string{"--set: profile.enforcement:", "advisory"}},
		{name: "unknown key", config: "version: 1\n", args: []string{"--set", "profile.gate.overal_min=80"},
			want: []string{"--set: profile.gate.overal_min: unknown property"}},
	})
}

func TestInterpolation(t *testing.T) {
	t.Parallel()
	const cfg = `version: 1
providers:
  local: { type: openai_compatible, base_url: "${LLM_HOST}/v1", api_key_env: LOCAL_KEY }
inputs:
  - { name: legacy, mode: run, format: json_mapping, command: ["${TOOLS}/scan", "--json"], mapping: { items: "$.i", rule_id: "$.r", message: "$.m", path: "$.p" } }
review_rules:
  - { id: r, description: "Never log ${SECRET} values", severity: high, category: security }
`
	t.Run("marked fields expand, others stay literal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeRepoConfig(t, dir, ".qgate.yaml", cfg)
		env := map[string]string{"LLM_HOST": "http://llm:11434", "TOOLS": "/opt/tools", "SECRET": "leak", "LOCAL_KEY": "sk-1"}
		res, doc := validateStage(t, clitest.Options{Dir: dir, Env: env})

		if got := dig(t, doc, "providers", "local", "base_url"); got != "http://llm:11434/v1" {
			t.Errorf("base_url = %v", got)
		}
		cmd := dig(t, doc, "inputs").([]any)[0].(map[string]any)["command"].([]any)
		if cmd[0] != "/opt/tools/scan" {
			t.Errorf("command[0] = %v", cmd[0])
		}
		rule := dig(t, doc, "review_rules").([]any)[0].(map[string]any)
		if rule["description"] != "Never log ${SECRET} values" {
			t.Errorf("description was interpolated: %v", rule["description"])
		}
		if strings.Contains(res.Stdout, "sk-1") {
			t.Error("API key value leaked into resolved config")
		}
	})
	t.Run("unset variable in a marked field is an error", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeRepoConfig(t, dir, ".qgate.yaml", cfg)
		res := clitest.Run(t, clitest.Options{Dir: dir, Env: map[string]string{"TOOLS": "/opt"}}, "validate")
		if res.Code != 2 || !strings.Contains(res.Stderr, ".qgate.yaml: providers.local.base_url: environment variable LLM_HOST is not set") {
			t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
		}
	})
}

func TestUnknownKeysAreReportedAtTheirOwnPath(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{name: "top level", config: "version: 1\nfoo: 1\n",
			want: []string{".qgate.yaml: foo: unknown property"}},
		{name: "nested", config: "version: 1\nproviders:\n  p:\n    type: anthropic\n    api_key: sk-1\n",
			want: []string{".qgate.yaml: providers.p.api_key: unknown property"}},
	})
}

func TestFallbackMustNameAKnownProvider(t *testing.T) {
	t.Parallel()
	runConfigErrorCases(t, []configErrorCase{
		{name: "unknown fallback provider",
			config: "version: 1\nproviders:\n  a: { type: anthropic, fallback: zzz/some-model }\n",
			want:   []string{".qgate.yaml: providers.a.fallback:", `unknown provider "zzz"`}},
	})
}

func TestSetErrorsReportOnlyTheRealProblem(t *testing.T) {
	t.Parallel()
	for _, set := range []string{"profile.gate.overall_min=abc", "profile.gate.max_new.critical=-1"} {
		t.Run(set, func(t *testing.T) {
			t.Parallel()
			res := clitest.Run(t, clitest.Options{}, "validate", "--set", set)
			if res.Code != 2 {
				t.Fatalf("exit code = %d, want 2; stderr:\n%s", res.Code, res.Stderr)
			}
			key := strings.SplitN(set, "=", 2)[0]
			for _, line := range strings.Split(strings.TrimSpace(res.Stderr), "\n")[1:] {
				if !strings.Contains(line, key) {
					t.Errorf("spurious error line %q for --set %s", line, set)
				}
			}
		})
	}
}

func TestSetCannotChangeTheStage(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{}, "validate", "--set", "stage=prod")
	if res.Code != 2 || !strings.Contains(res.Stderr, "--stage") {
		t.Errorf("exit code = %d, want 2 pointing at --stage; stderr:\n%s", res.Code, res.Stderr)
	}
}
