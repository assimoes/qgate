package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qgate/internal/clitest"
)

const orgConfig = `version: 1
scoring:
  agent_max_penalty: 10
  category_weights: { security: 50 }
providers:
  local: { type: openai_compatible, base_url: "http://llm" }
agents:
  sec:
    kind: reviewer
    provider: local
    model: org-model
    prompt: { base: builtin://security-reviewer@1 }
    categories: [security]
inputs:
  - { name: semgrep, format: sarif, path: org/semgrep.sarif, category_default: security, required: true }
  - { name: trivy, format: sarif, path: org/trivy.sarif }
review_rules:
  - { id: no-raw-sql, description: org text, severity: high, category: security }
suppressions:
  - { rule: "org:*", reason: org reason }
profiles:
  prod: { enforcement: blocking, gate: { overall_min: 90 } }
`

func TestOrgConfigIsLayeredBetweenBuiltinAndRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, "org.yaml", orgConfig)
	writeRepoConfig(t, dir, ".qgate.yaml", `version: 1
scoring: { agent_max_penalty: 12 }
profiles:
  prod: { gate: { overall_min: 95 } }
`)
	_, doc := validateStage(t, clitest.Options{Dir: dir}, "--org-config", "org.yaml", "--stage", "prod")

	for _, c := range []struct {
		path []string
		want any
	}{
		{[]string{"scoring", "agent_max_penalty"}, 12},            // repo over org
		{[]string{"scoring", "category_weights", "security"}, 50}, // org over builtin
		{[]string{"scoring", "category_weights", "quality"}, 30},  // builtin, untouched
		{[]string{"profile", "enforcement"}, "blocking"},          // org profile
		{[]string{"profile", "gate", "overall_min"}, 95},          // repo profile over org profile
		{[]string{"agents", "sec", "model"}, "org-model"},         // org-only agent
	} {
		if got := dig(t, doc, c.path...); got != c.want {
			t.Errorf("%s = %v, want %v", strings.Join(c.path, "."), got, c.want)
		}
	}
}

func TestListMergeRulesBetweenOrgAndRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, "org.yaml", orgConfig)
	writeRepoConfig(t, dir, ".qgate.yaml", `version: 1
agents:
  sec: { model: repo-model }
inputs:
  - { name: semgrep, path: reports/semgrep.sarif }
  - { name: golangci, format: checkstyle, path: reports/golangci.xml }
review_rules:
  - { id: no-raw-sql, description: repo text, severity: high, category: security }
  - { id: no-panics, description: d, severity: medium, category: quality }
suppressions:
  - { rule: "repo:*", reason: repo reason }
`)
	_, doc := validateStage(t, clitest.Options{Dir: dir}, "--org-config", "org.yaml")

	inputs := dig(t, doc, "inputs").([]any)
	if len(inputs) != 3 {
		t.Fatalf("inputs = %v, want 3 merged by name", inputs)
	}
	semgrep := inputs[0].(map[string]any)
	if semgrep["name"] != "semgrep" || semgrep["path"] != "reports/semgrep.sarif" || semgrep["required"] != true {
		t.Errorf("semgrep = %v, want repo path merged into org entry (required kept)", semgrep)
	}
	if inputs[1].(map[string]any)["name"] != "trivy" || inputs[2].(map[string]any)["name"] != "golangci" {
		t.Errorf("inputs order = %v, want org entries then new repo entries", inputs)
	}

	rules := dig(t, doc, "review_rules").([]any)
	if len(rules) != 2 || rules[0].(map[string]any)["description"] != "repo text" {
		t.Errorf("review_rules = %v, want 2 merged by id with repo text", rules)
	}

	sup := dig(t, doc, "suppressions").([]any)
	if len(sup) != 2 || sup[0].(map[string]any)["rule"] != "org:*" || sup[1].(map[string]any)["rule"] != "repo:*" {
		t.Errorf("suppressions = %v, want org then repo appended", sup)
	}

	agent := dig(t, doc, "agents", "sec").(map[string]any)
	if agent["model"] != "repo-model" || agent["provider"] != "local" {
		t.Errorf("agents.sec = %v, want repo model deep-merged over org agent", agent)
	}
}

// orgServer serves body over HTTPS and returns its URL and an HTTP client
// that trusts it.
func orgServer(t *testing.T, status int, body string) (string, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/qgate/org.yaml", srv.Client()
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestOrgConfigFromHTTPS(t *testing.T) {
	t.Parallel()
	url, client := orgServer(t, http.StatusOK, orgConfig)

	t.Run("unpinned", func(t *testing.T) {
		t.Parallel()
		_, doc := validateStage(t, clitest.Options{HTTPClient: client}, "--org-config", url)
		if got := dig(t, doc, "scoring", "agent_max_penalty"); got != 10 {
			t.Errorf("scoring.agent_max_penalty = %v, want 10 from the org config", got)
		}
	})
	t.Run("pinned and matching", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeRepoConfig(t, dir, ".qgate.yaml", "version: 1\norg_config_sha256: "+sha256Hex(orgConfig)+"\n")
		validateStage(t, clitest.Options{Dir: dir, HTTPClient: client}, "--org-config", url)
	})
	t.Run("pinned and mismatched", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeRepoConfig(t, dir, ".qgate.yaml", "version: 1\norg_config_sha256: "+sha256Hex("something else")+"\n")
		res := clitest.Run(t, clitest.Options{Dir: dir, HTTPClient: client}, "validate", "--org-config", url)
		if res.Code != 2 || !strings.Contains(res.Stderr, "org_config_sha256") || !strings.Contains(res.Stderr, sha256Hex(orgConfig)) {
			t.Errorf("exit code = %d, want 2 naming the pin and the actual digest; stderr:\n%s", res.Code, res.Stderr)
		}
	})
}

func TestOrgConfigSourceErrors(t *testing.T) {
	t.Parallel()
	notFoundURL, notFoundClient := orgServer(t, http.StatusNotFound, "")
	tests := []struct {
		name   string
		args   []string
		client *http.Client
		code   int
		want   string
	}{
		{"plain http is refused", []string{"--org-config", "http://example.invalid/org.yaml"}, nil, 2, "HTTPS"},
		{"missing file", []string{"--org-config", "nope.yaml"}, nil, 2, "nope.yaml: org config not found"},
		{"server error is an infrastructure failure", []string{"--org-config", notFoundURL}, notFoundClient, 3, "404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := clitest.Run(t, clitest.Options{HTTPClient: tt.client}, append([]string{"validate"}, tt.args...)...)
			if res.Code != tt.code || !strings.Contains(res.Stderr, tt.want) {
				t.Errorf("exit code = %d, want %d with %q; stderr:\n%s", res.Code, tt.code, tt.want, res.Stderr)
			}
		})
	}
}

func TestOrgConfigIsValidatedAsItsOwnFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, "org.yaml", "version: 1\nscoring: { agent_weight_pct: lots }\n")
	res := clitest.Run(t, clitest.Options{Dir: dir}, "validate", "--org-config", "org.yaml")
	if res.Code != 2 || !strings.Contains(res.Stderr, "org.yaml: scoring.agent_weight_pct:") {
		t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
	}
}

const lockingOrg = `version: 1
locked: [scoring.agent_max_penalty, llm.allowed_providers, profile.enforcement]
scoring: { agent_max_penalty: 10 }
llm: { allowed_providers: [local] }
profiles:
  prod: { enforcement: blocking }
`

func TestLockedKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		repo  string
		args  []string
		code  int
		wants []string
	}{
		{name: "repo leaves locked keys alone", repo: "version: 1\nproject: { name: x }\n", code: 0},
		{name: "repo restates the locked value", repo: "version: 1\nscoring: { agent_max_penalty: 10 }\n", code: 0},
		{name: "repo config changes a locked key",
			repo:  "version: 1\nscoring: { agent_max_penalty: 40 }\n",
			code:  2,
			wants: []string{".qgate.yaml: scoring.agent_max_penalty: locked by the org config", "10"}},
		{name: "repo profile changes a locked key",
			repo:  "version: 1\nprofiles:\n  prod: { enforcement: advisory }\n",
			args:  []string{"--stage", "prod"},
			code:  2,
			wants: []string{".qgate.yaml: profile.enforcement: locked by the org config", `"blocking"`}},
		{name: "--set changes a locked key",
			repo:  "version: 1\n",
			args:  []string{"--set", "llm.allowed_providers=[anthropic]"},
			code:  2,
			wants: []string{"--set: llm.allowed_providers: locked by the org config", `["local"]`}},
		{name: "--set may change an unlocked key", repo: "version: 1\n",
			args: []string{"--set", "scoring.agent_weight_pct=40"}, code: 0},
		{name: "only the org config may lock keys",
			repo:  "version: 1\nlocked: [scoring]\n",
			code:  2,
			wants: []string{".qgate.yaml: locked: only the org config may lock keys"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeRepoConfig(t, dir, "org.yaml", lockingOrg)
			writeRepoConfig(t, dir, ".qgate.yaml", tt.repo)
			res := clitest.Run(t, clitest.Options{Dir: dir}, append([]string{"validate", "--org-config", "org.yaml"}, tt.args...)...)
			if res.Code != tt.code {
				t.Fatalf("exit code = %d, want %d; stderr:\n%s", res.Code, tt.code, res.Stderr)
			}
			for _, w := range tt.wants {
				if !strings.Contains(res.Stderr, w) {
					t.Errorf("stderr missing %q:\n%s", w, res.Stderr)
				}
			}
		})
	}
}

func TestValidateShowsLockedKeys(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, "org.yaml", "version: 1\nlocked: [scoring.agent_max_penalty, scoring.agent_weight_pct]\nscoring: { agent_max_penalty: 10 }\n")

	res, _ := validateStage(t, clitest.Options{Dir: dir}, "--org-config", "org.yaml")
	for _, w := range []string{
		"# Locked by the org config org.yaml:",
		"#   scoring.agent_max_penalty = 10 (set by org.yaml)",
		"#   scoring.agent_weight_pct = 50 (set by builtin defaults)",
	} {
		if !strings.Contains(res.Stdout, w) {
			t.Errorf("stdout missing %q:\n%s", w, res.Stdout)
		}
	}
}

func TestLockedKeyMustBeAConfigPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, "org.yaml", "version: 1\nlocked: [scoring.agent_max_penatly]\n")
	res := clitest.Run(t, clitest.Options{Dir: dir}, "validate", "--org-config", "org.yaml")
	if res.Code != 2 || !strings.Contains(res.Stderr, `org.yaml: locked[0]: "scoring.agent_max_penatly" is not a config path`) {
		t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
	}
}

func TestErrorsNameTheLayerThatCausedThem(t *testing.T) {
	t.Parallel()
	runOrg := func(t *testing.T, org, repo string) clitest.Result {
		t.Helper()
		dir := t.TempDir()
		writeRepoConfig(t, dir, "org.yaml", org)
		writeRepoConfig(t, dir, ".qgate.yaml", repo)
		return clitest.Run(t, clitest.Options{Dir: dir}, "validate", "--org-config", "org.yaml")
	}
	t.Run("semantic error in the org config", func(t *testing.T) {
		t.Parallel()
		res := runOrg(t, "version: 1\ninputs:\n  - { name: a, format: sarif, path: a, category_default: secrity }\n",
			"version: 1\ninputs:\n  - { name: b, format: sarif, path: b }\n")
		if res.Code != 2 || !strings.Contains(res.Stderr, "org.yaml: inputs[0].category_default:") {
			t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
		}
	})
	t.Run("semantic error in a repo entry merged over an org entry", func(t *testing.T) {
		t.Parallel()
		res := runOrg(t, "version: 1\ninputs:\n  - { name: a, format: sarif, path: a }\n",
			"version: 1\ninputs:\n  - { name: a, category_default: secrity }\n")
		if res.Code != 2 || !strings.Contains(res.Stderr, ".qgate.yaml: inputs[0].category_default:") {
			t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
		}
	})
	t.Run("required field missing after merging", func(t *testing.T) {
		t.Parallel()
		res := runOrg(t, "version: 1\n", "version: 1\nagents:\n  sec: { model: m }\n")
		if res.Code != 2 || !strings.Contains(res.Stderr, ".qgate.yaml: agents.sec:") || !strings.Contains(res.Stderr, "kind") {
			t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
		}
	})
	t.Run("org config may not pin itself", func(t *testing.T) {
		t.Parallel()
		res := runOrg(t, "version: 1\norg_config_sha256: "+strings.Repeat("a", 64)+"\n", "version: 1\n")
		if res.Code != 2 || !strings.Contains(res.Stderr, "org.yaml: org_config_sha256: only a repo config may pin") {
			t.Errorf("exit code = %d, stderr:\n%s", res.Code, res.Stderr)
		}
	})
}
