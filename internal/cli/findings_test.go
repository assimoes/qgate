package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"qgate/internal/clitest"
	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
)

// sarifResult is one result in a hand-built SARIF log.
type sarifResult struct {
	rule      string
	level     string // "" omits it
	path      string
	line      int // 0 omits the region (file-level)
	endLine   int // 0 omits it
	secSev    string
	partialFP string
	cwe       string
	kind      string // "" omits it (SARIF default "fail")
}

// sarifLog builds a SARIF 2.1.0 log with one run per tool name.
func sarifLog(runs map[string][]sarifResult) string {
	var outRuns []any
	for _, tool := range sortedTools(runs) {
		rules := map[string]any{}
		var results []any
		for _, r := range runs[tool] {
			rule := map[string]any{"id": r.rule, "shortDescription": map[string]any{"text": "title of " + r.rule}}
			props := map[string]any{}
			if r.secSev != "" {
				props["security-severity"] = r.secSev
			}
			if r.cwe != "" {
				props["tags"] = []any{"security", "external/cwe/" + strings.ToLower(r.cwe)}
			}
			if len(props) > 0 {
				rule["properties"] = props
			}
			rules[r.rule] = rule

			phys := map[string]any{"artifactLocation": map[string]any{"uri": r.path}}
			if r.line > 0 {
				region := map[string]any{"startLine": r.line}
				if r.endLine > 0 {
					region["endLine"] = r.endLine
				}
				phys["region"] = region
			}
			res := map[string]any{
				"ruleId":    r.rule,
				"message":   map[string]any{"text": "message for " + r.rule},
				"locations": []any{map[string]any{"physicalLocation": phys}},
			}
			if r.level != "" {
				res["level"] = r.level
			}
			if r.kind != "" {
				res["kind"] = r.kind
			}
			if r.partialFP != "" {
				res["partialFingerprints"] = map[string]any{"primaryLocationLineHash": r.partialFP}
			}
			results = append(results, res)
		}
		var ruleList []any
		for _, id := range sortedKeysOf(rules) {
			ruleList = append(ruleList, rules[id])
		}
		outRuns = append(outRuns, map[string]any{
			"tool":    map[string]any{"driver": map[string]any{"name": tool, "version": "1.2.3", "rules": ruleList}},
			"results": results,
		})
	}
	data, _ := json.MarshalIndent(map[string]any{
		"$schema": "https://json.schemastore.org/sarif-2.1.0.json",
		"version": "2.1.0",
		"runs":    outRuns,
	}, "", "  ")
	return string(data)
}

func sortedTools(m map[string][]sarifResult) []string {
	return slices.Sorted(maps.Keys(m))
}

func sortedKeysOf(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

const semgrepInput = `version: 1
inputs:
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif, category_default: security }
`

// history builds a base and a head commit covering added, modified,
// renamed, deleted and binary files.
func history(t *testing.T) (repo *clitest.Repo, base, head string) {
	t.Helper()
	repo = clitest.NewRepo(t)
	repo.Write("a.go", clitest.Lines(10))
	repo.Write("same.go", clitest.Lines(5))
	repo.Write("old.go", clitest.Lines(5))
	repo.Write("gone.go", clitest.Lines(3))
	repo.Write("bin.dat", "\x00\x01\x02binary\x00")
	repo.Write("vendor/lib.go", clitest.Lines(3))
	base = repo.Commit("base")

	repo.Write("a.go", strings.Replace(clitest.Lines(10), "line 5\n", "line 5 changed\n", 1))
	// Distinct from gone.go, or git would detect gone.go → new.go as a rename.
	repo.Write("new.go", "package n\n\nfunc New() {}\n")
	repo.Rename("old.go", "renamed.go")
	repo.Remove("gone.go")
	repo.Write("bin.dat", "\x00\x01\x02binary v2\x00")
	repo.Write("vendor/lib.go", strings.Replace(clitest.Lines(3), "line 1\n", "line 1 changed\n", 1))
	head = repo.Commit("head")
	return repo, base, head
}

// scopeResults are findings on every kind of location in history.
var scopeResults = []sarifResult{
	{rule: "changed-line", level: "error", path: "a.go", line: 5},
	{rule: "unchanged-line", level: "error", path: "a.go", line: 2},
	{rule: "file-level-changed", level: "error", path: "a.go"},
	{rule: "file-level-unchanged", level: "error", path: "same.go"},
	{rule: "unchanged-file", level: "error", path: "same.go", line: 3},
	{rule: "added-file", level: "error", path: "new.go", line: 1},
	{rule: "pure-rename", level: "error", path: "renamed.go", line: 2},
	{rule: "binary", level: "error", path: "bin.dat", line: 1},
	{rule: "vendored", level: "error", path: "vendor/lib.go", line: 1},
}

// findingsByRule runs qgate and indexes the report's findings by rule ID.
func findingsByRule(t *testing.T, res clitest.Result) map[string]map[string]any {
	t.Helper()
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	validateReport(t, res.Report)
	out := map[string]map[string]any{}
	for _, f := range res.DecodeReport(t)["findings"].([]any) {
		m := f.(map[string]any)
		out[m["rule_id"].(string)] = m
	}
	return out
}

func TestGatingScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scope string
		new   []string // rule names expected new; the rest are out_of_scope
	}{
		{"diff", []string{"changed-line", "file-level-changed", "added-file"}},
		{"files", []string{"changed-line", "unchanged-line", "file-level-changed", "added-file", "pure-rename", "binary"}},
		{"repo", []string{"changed-line", "unchanged-line", "file-level-changed", "file-level-unchanged",
			"unchanged-file", "added-file", "pure-rename", "binary"}},
	}
	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			t.Parallel()
			repo, base, _ := history(t)
			repo.Write(".qgate.yaml", semgrepInput+"project: { exclude: [\"vendor/**\"] }\n")
			repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults}))

			found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir},
				"run", "--base", base, "--set", "profile.scope="+tt.scope))

			if _, ok := found["semgrep:vendored"]; ok {
				t.Error("finding in an excluded path was not dropped")
			}
			isNew := map[string]bool{}
			for _, r := range tt.new {
				isNew["semgrep:"+r] = true
			}
			for _, r := range scopeResults {
				if r.rule == "vendored" {
					continue
				}
				id := "semgrep:" + r.rule
				f, ok := found[id]
				if !ok {
					t.Errorf("%s missing from report", id)
					continue
				}
				want := "out_of_scope"
				if isNew[id] {
					want = "new"
				}
				if f["status"] != want {
					t.Errorf("%s status = %v, want %s", id, f["status"], want)
				}
			}
		})
	}
}

func TestBaseAndHeadResolution(t *testing.T) {
	t.Parallel()
	t.Run("merge-base with origin's default branch when --base is absent", func(t *testing.T) {
		t.Parallel()
		repo, base, head := history(t)
		repo.SetOrigin(base)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults[:2]}))

		res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run")
		found := findingsByRule(t, res)
		run := res.DecodeReport(t)["run"].(map[string]any)
		if run["base_sha"] != base || run["head_sha"] != head {
			t.Errorf("run base/head = %v/%v, want %s/%s", run["base_sha"], run["head_sha"], base, head)
		}
		if found["semgrep:changed-line"]["status"] != "new" || found["semgrep:unchanged-line"]["status"] != "out_of_scope" {
			t.Errorf("statuses wrong with merge-base fallback: %v", found)
		}
	})
	t.Run("explicit --head", func(t *testing.T) {
		t.Parallel()
		repo, base, head := history(t)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults[:2]}))
		repo.Write("later.go", "x\n")
		repo.Commit("later, not under review")

		res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--head", head)
		findingsByRule(t, res)
		if got := res.DecodeReport(t)["run"].(map[string]any)["head_sha"]; got != head {
			t.Errorf("run.head_sha = %v, want %s", got, head)
		}
	})
	t.Run("no base and no origin is a usage error", func(t *testing.T) {
		t.Parallel()
		repo, _, _ := history(t)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults[:1]}))
		res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run")
		if res.Code != 2 || !strings.Contains(res.Stderr, "--base") {
			t.Errorf("exit code = %d, want 2 suggesting --base; stderr:\n%s", res.Code, res.Stderr)
		}
	})
}

func TestSARIFNormalization(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", semgrepInput)
	results := []sarifResult{
		{rule: "lvl-error", level: "error", path: "a.go", line: 5},
		{rule: "lvl-warning", level: "warning", path: "a.go", line: 5},
		{rule: "lvl-note", level: "note", path: "a.go", line: 5},
		{rule: "lvl-none", level: "none", path: "a.go", line: 5},
		{rule: "lvl-default", path: "a.go", line: 5},
		{rule: "lvl-bogus", level: "bogus", path: "a.go", line: 5},
		{rule: "sec-9.8", level: "note", secSev: "9.8", path: "a.go", line: 5, cwe: "CWE-89"},
		{rule: "sec-7.0", secSev: "7.0", path: "a.go", line: 5},
		{rule: "sec-4.0", secSev: "4.0", path: "a.go", line: 5},
		{rule: "sec-1.0", secSev: "1.0", path: "a.go", line: 5},
	}
	repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{
		"Semgrep":   results,
		"OtherTool": {{rule: "second-run", level: "warning", path: "new.go", line: 1}},
	}))

	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	found := findingsByRule(t, res)

	wantSeverity := map[string]string{
		"lvl-error": "high", "lvl-warning": "medium", "lvl-note": "low", "lvl-none": "info",
		"lvl-default": "medium", "lvl-bogus": "medium",
		"sec-9.8": "critical", "sec-7.0": "high", "sec-4.0": "medium", "sec-1.0": "low",
		"second-run": "medium",
	}
	for rule, want := range wantSeverity {
		f, ok := found["semgrep:"+rule]
		if !ok {
			t.Errorf("semgrep:%s missing (rule IDs are namespaced by input name, every run is ingested)", rule)
			continue
		}
		if f["severity"] != want {
			t.Errorf("semgrep:%s severity = %v, want %s", rule, f["severity"], want)
		}
	}

	f := found["semgrep:sec-9.8"]
	if cwe, _ := f["cwe"].([]any); len(cwe) != 1 || cwe[0] != "CWE-89" {
		t.Errorf("cwe = %v, want [CWE-89]", f["cwe"])
	}
	if f["category"] != "security" || f["title"] != "title of sec-9.8" || f["message"] != "message for sec-9.8" {
		t.Errorf("category/title/message = %v / %v / %v", f["category"], f["title"], f["message"])
	}
	src := f["source"].(map[string]any)
	if src["kind"] != "tool" || src["name"] != "semgrep" || src["version"] != "1.2.3" {
		t.Errorf("source = %v", src)
	}

	inputs := res.DecodeReport(t)["inputs"].([]any)
	in := inputs[0].(map[string]any)
	if in["name"] != "semgrep" || in["status"] != "ok" || in["findings"] != float64(len(results)+1) {
		t.Errorf("input status = %v", in)
	}
	warnings := fmt.Sprint(in["warnings"])
	if !strings.Contains(warnings, "bogus") || !strings.Contains(warnings, "medium") {
		t.Errorf("warnings = %v, want one about the unmapped level", warnings)
	}
}

func TestDefaultCategoryIsQuality(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", "version: 1\ninputs:\n  - { name: lint, format: sarif, path: lint.sarif }\n")
	repo.Write("lint.sarif", sarifLog(map[string][]sarifResult{"Lint": {{rule: "r", level: "warning", path: "a.go", line: 5}}}))
	found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base))
	if got := found["lint:r"]["category"]; got != "quality" {
		t.Errorf("category = %v, want quality", got)
	}
}

func TestFingerprints(t *testing.T) {
	t.Parallel()
	fingerprint := func(t *testing.T, repo *clitest.Repo, base string, results []sarifResult) map[string]string {
		t.Helper()
		repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": results}))
		found := findingsByRule(t, clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--set", "profile.scope=repo"))
		out := map[string]string{}
		for id, f := range found {
			out[id] = f["fingerprint"].(string)
		}
		return out
	}

	t.Run("stable when lines are inserted above", func(t *testing.T) {
		t.Parallel()
		repo := clitest.NewRepo(t)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("f.go", "package f\n\nfunc risky() { exec(input) }\n")
		base := repo.Commit("one")
		before := fingerprint(t, repo, base, []sarifResult{{rule: "exec", level: "error", path: "f.go", line: 3}})

		repo.Write("f.go", "package f\n\n// a\n// b\n// c\nfunc risky() { exec(input) }\n")
		repo.Commit("insert above")
		after := fingerprint(t, repo, base, []sarifResult{{rule: "exec", level: "error", path: "f.go", line: 6}})

		if before["semgrep:exec"] == "" || before["semgrep:exec"] != after["semgrep:exec"] {
			t.Errorf("fingerprint changed: %s → %s", before["semgrep:exec"], after["semgrep:exec"])
		}
	})
	t.Run("identical lines in one file get distinct fingerprints", func(t *testing.T) {
		t.Parallel()
		repo := clitest.NewRepo(t)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("f.go", "dup()\nother()\ndup()\n")
		base := repo.Commit("one")
		repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
			{rule: "dup", level: "error", path: "f.go", line: 1},
			{rule: "dup", level: "error", path: "f.go", line: 3},
		}}))
		res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--set", "profile.scope=repo")
		if res.Code != 0 {
			t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
		}
		fs := res.DecodeReport(t)["findings"].([]any)
		if len(fs) != 2 || fs[0].(map[string]any)["fingerprint"] == fs[1].(map[string]any)["fingerprint"] {
			t.Errorf("findings = %v, want two with distinct fingerprints", fs)
		}
	})
	t.Run("analyzer fingerprints are used when present", func(t *testing.T) {
		t.Parallel()
		repo := clitest.NewRepo(t)
		repo.Write(".qgate.yaml", semgrepInput)
		repo.Write("f.go", "a()\nb()\n")
		base := repo.Commit("one")
		got := fingerprint(t, repo, base, []sarifResult{
			{rule: "x", level: "error", path: "f.go", line: 1, partialFP: "tool-fp-1"},
		})
		repo.Write("f.go", "a(changed)\nb()\n")
		repo.Commit("change the line")
		again := fingerprint(t, repo, base, []sarifResult{
			{rule: "x", level: "error", path: "f.go", line: 1, partialFP: "tool-fp-1"},
		})
		if got["semgrep:x"] != again["semgrep:x"] {
			t.Errorf("fingerprint followed line content despite a stable analyzer fingerprint")
		}
	})
}

func TestMissingImportFileFailsTheInput(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", semgrepInput)
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0 (failure policy is ticket 09); stderr:\n%s", res.Code, res.Stderr)
	}
	in := res.DecodeReport(t)["inputs"].([]any)[0].(map[string]any)
	if in["status"] != "failed" || !strings.Contains(fmt.Sprint(in["error"]), "semgrep.sarif") {
		t.Errorf("input = %v, want failed naming the missing file", in)
	}
}

func TestSARIFResultsThatAreNotDefectsAreSkipped(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", semgrepInput)
	repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": {
		{rule: "defect", level: "error", path: "a.go", line: 5},
		{rule: "explicit-fail", kind: "fail", path: "a.go", line: 5},
		{rule: "passed", kind: "pass", path: "a.go", line: 5},
		{rule: "not-applicable", kind: "notApplicable", path: "a.go", line: 5},
		{rule: "info-only", kind: "informational", path: "a.go", line: 5},
	}}))
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	found := findingsByRule(t, res)
	if len(found) != 2 || found["semgrep:defect"] == nil || found["semgrep:explicit-fail"] == nil {
		t.Errorf("findings = %v, want only the two defects", keys(found))
	}
	in := res.DecodeReport(t)["inputs"].([]any)[0].(map[string]any)
	if !strings.Contains(fmt.Sprint(in["warnings"]), "3 results") {
		t.Errorf("warnings = %v, want a count of skipped non-defect results", in["warnings"])
	}
}

func keys(m map[string]map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func TestSARIFLocationsAreURIs(t *testing.T) {
	t.Parallel()
	repo := clitest.NewRepo(t)
	repo.Write(".qgate.yaml", semgrepInput)
	repo.Write("my file.go", "a\n")
	repo.Write("sub/x.go", "b\n")
	base := repo.Commit("one")
	log := `{"version": "2.1.0", "runs": [{
	  "tool": {"driver": {"name": "T", "rules": [{"id": "r"}]}},
	  "originalUriBaseIds": {"SRC": {"uri": "file://` + repo.Dir + `/sub/"}},
	  "results": [
	    {"ruleId": "r", "message": {"text": "encoded"}, "locations": [{"physicalLocation": {
	      "artifactLocation": {"uri": "my%20file.go"}, "region": {"startLine": 1}}}]},
	    {"ruleId": "r", "message": {"text": "base id"}, "locations": [{"physicalLocation": {
	      "artifactLocation": {"uri": "x.go", "uriBaseId": "SRC"}, "region": {"startLine": 1}}}]}
	  ]}]}`
	repo.Write("reports/semgrep.sarif", log)

	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base, "--set", "profile.scope=repo")
	if res.Code != 0 {
		t.Fatalf("exit code = %d; stderr:\n%s", res.Code, res.Stderr)
	}
	paths := map[string]bool{}
	for _, f := range res.DecodeReport(t)["findings"].([]any) {
		paths[f.(map[string]any)["location"].(map[string]any)["path"].(string)] = true
	}
	if !paths["my file.go"] || !paths["sub/x.go"] {
		t.Errorf("paths = %v, want \"my file.go\" (percent-decoded) and \"sub/x.go\" (uriBaseId resolved)", paths)
	}
}

func TestRunOutsideAGitRepositoryIsAUsageError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeRepoConfig(t, dir, ".qgate.yaml", semgrepInput)
	res := clitest.Run(t, clitest.Options{Dir: dir}, "run", "--base", "HEAD")
	if res.Code != 2 || !strings.Contains(res.Stderr, "not inside a git repository") {
		t.Errorf("exit code = %d, want 2; stderr:\n%s", res.Code, res.Stderr)
	}
}

func TestOneFailingInputDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", `version: 1
inputs:
  - { name: missing, format: sarif, path: nope.sarif }
  - { name: semgrep, format: sarif, path: reports/semgrep.sarif }
`)
	repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults[:1]}))
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir}, "run", "--base", base)
	found := findingsByRule(t, res)
	if found["semgrep:changed-line"] == nil {
		t.Errorf("the healthy input's findings are missing: %v", keys(found))
	}
}

// blockingIngestor parses nothing until its context is cancelled, to test
// that cancellation reaches inputs in flight.
type blockingIngestor struct{ started chan struct{} }

func (blockingIngestor) Format() string { return "sarif" }

func (b blockingIngestor) Parse(ctx context.Context, _ io.Reader, _ config.Input, _ *gitctx.Context) (ingest.Result, error) {
	b.started <- struct{}{}
	<-ctx.Done()
	return ingest.Result{}, ctx.Err()
}

func TestCancellationStopsInputsInFlight(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	var cfg strings.Builder
	cfg.WriteString("version: 1\ninputs:\n")
	for i := range 6 { // more than run in parallel at once
		fmt.Fprintf(&cfg, "  - { name: in%d, format: sarif, path: reports/semgrep.sarif }\n", i)
	}
	repo.Write(".qgate.yaml", cfg.String())
	repo.Write("reports/semgrep.sarif", sarifLog(map[string][]sarifResult{"Semgrep": scopeResults[:1]}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 6)
	go func() {
		<-started // an input is in flight
		cancel()
	}()

	done := make(chan clitest.Result, 1)
	go func() {
		done <- clitest.Run(t, clitest.Options{
			Dir:       repo.Dir,
			Context:   ctx,
			Ingestors: ingest.NewRegistry(blockingIngestor{started: started}),
		}, "run", "--base", base)
	}()
	select {
	case res := <-done:
		if res.Code != 3 || !strings.Contains(res.Stderr, "context canceled") {
			t.Errorf("exit code = %d, want 3 (cancelled run); stderr:\n%s", res.Code, res.Stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not stop after cancellation")
	}
}

func TestCancelledRunIsNotMistakenForAMissingRepository(t *testing.T) {
	t.Parallel()
	repo, base, _ := history(t)
	repo.Write(".qgate.yaml", semgrepInput)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := clitest.Run(t, clitest.Options{Dir: repo.Dir, Context: ctx}, "run", "--base", base)
	if res.Code != 3 || strings.Contains(res.Stderr, "not inside a git repository") {
		t.Errorf("exit code = %d, want 3 for cancellation; stderr:\n%s", res.Code, res.Stderr)
	}
}
