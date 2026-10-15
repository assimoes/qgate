package gate_test

import (
	"testing"

	"qgate/internal/config"
	"qgate/internal/gate"
	"qgate/internal/model"
	"qgate/internal/score"
)

func h(v int) *int { return &v }

func intp(v int) *int { return &v }

func newFinding(sev model.Severity, kind model.SourceKind, conf *int) model.Finding {
	return model.Finding{Severity: sev, Status: model.StatusNew, Source: model.Source{Kind: kind}, Confidence: conf}
}

// checkByName indexes a result's gate checks.
func checkByName(t *testing.T, r model.GateResult) map[string]model.GateCheck {
	t.Helper()
	out := map[string]model.GateCheck{}
	for _, c := range r.Checks {
		if _, dup := out[c.Check]; dup {
			t.Fatalf("gate check %s listed twice", c.Check)
		}
		out[c.Check] = c
	}
	return out
}

func TestGateChecks(t *testing.T) {
	t.Parallel()
	scores := score.Scores{
		Overall: h(7650),
		Categories: map[model.Category]*int{
			"security": h(7600), "quality": h(9000), "testing": nil, "maintainability": nil,
		},
	}
	findings := []model.Finding{
		newFinding(model.SeverityHigh, model.SourceTool, nil),
		newFinding(model.SeverityHigh, model.SourceAgent, intp(90)),
		newFinding(model.SeverityHigh, model.SourceAgent, intp(10)), // below min confidence: not counted
		newFinding(model.SeverityCritical, model.SourceAgent, intp(95)),
		{Severity: model.SeverityCritical, Status: model.StatusOutOfScope, Source: model.Source{Kind: model.SourceTool}},
	}
	g := config.Gate{
		OverallMin:      h(76),
		CategoryMin:     map[model.Category]int{"security": 77, "quality": 90, "testing": 50, "maintainability": 50},
		MaxNew:          map[model.Severity]int{model.SeverityHigh: 2, model.SeverityCritical: 0},
		MaxNewTool:      map[model.Severity]int{model.SeverityHigh: 1, model.SeverityCritical: 0},
		RequireAssessed: []model.Category{"quality", "testing"},
	}
	res := gate.Evaluate(gate.Input{
		Scores: scores, Findings: findings, Gate: g,
		Enforcement: model.EnforcementBlocking, AgentMinConfidence: 70,
	})
	checks := checkByName(t, res)

	want := []struct {
		name     string
		result   model.CheckResult
		expected string
		actual   any
	}{
		{"overall_min", model.CheckPass, ">= 76", 76},                      // 76.50 ≥ 76
		{"category_min.security", model.CheckFail, ">= 77", 76},            // 76.00 < 77
		{"category_min.quality", model.CheckPass, ">= 90", 90},             // equal passes
		{"category_min.testing", model.CheckFail, ">= 50", nil},            // not assessed but required
		{"category_min.maintainability", model.CheckSkipped, ">= 50", nil}, // not assessed, not required
		{"max_new.high", model.CheckPass, "<= 2", 2},                       // tool + confident agent
		{"max_new.critical", model.CheckFail, "<= 0", 1},                   // confident agent; out-of-scope ignored
		{"max_new_tool.high", model.CheckPass, "<= 1", 1},
		{"max_new_tool.critical", model.CheckPass, "<= 0", 0},
		{"require_assessed.quality", model.CheckPass, "assessed", true},
		{"require_assessed.testing", model.CheckFail, "assessed", false},
	}
	for _, w := range want {
		c, ok := checks[w.name]
		if !ok {
			t.Errorf("gate check %s missing", w.name)
			continue
		}
		if c.Result != w.result || c.Expected != w.expected || c.Actual != w.actual {
			t.Errorf("%s = {%s %q %v}, want {%s %q %v}", w.name, c.Result, c.Expected, c.Actual, w.result, w.expected, w.actual)
		}
	}
	if len(checks) != len(want) {
		t.Errorf("got %d gate checks, want %d", len(checks), len(want))
	}
	if checks["category_min.maintainability"].Note == "" || checks["category_min.testing"].Note == "" {
		t.Error("not-assessed category checks should explain themselves in a note")
	}
	if res.Outcome != model.OutcomeFail {
		t.Errorf("outcome = %s, want fail", res.Outcome)
	}
}

func TestOverallMinFailsWhenNothingIsAssessed(t *testing.T) {
	t.Parallel()
	res := gate.Evaluate(gate.Input{
		Scores: score.Scores{Categories: map[model.Category]*int{}},
		Gate:   config.Gate{OverallMin: h(1)}, Enforcement: model.EnforcementBlocking,
	})
	c := checkByName(t, res)["overall_min"]
	if c.Result != model.CheckFail || c.Actual != nil {
		t.Errorf("overall_min = %+v, want fail with null actual", c)
	}
}

func TestOutcome(t *testing.T) {
	t.Parallel()
	violated := config.Gate{MaxNew: map[model.Severity]int{model.SeverityHigh: 0}}
	high := []model.Finding{newFinding(model.SeverityHigh, model.SourceTool, nil)}
	tests := []struct {
		name        string
		enforcement model.Enforcement
		findings    []model.Finding
		degraded    bool
		want        model.Outcome
	}{
		{"blocking, clean", model.EnforcementBlocking, nil, false, model.OutcomePass},
		{"blocking, violation", model.EnforcementBlocking, high, false, model.OutcomeFail},
		{"blocking, degraded only", model.EnforcementBlocking, nil, true, model.OutcomeWarn},
		{"blocking, violation and degraded", model.EnforcementBlocking, high, true, model.OutcomeFail},
		{"advisory, clean", model.EnforcementAdvisory, nil, false, model.OutcomePass},
		{"advisory, violation", model.EnforcementAdvisory, high, false, model.OutcomeWarn},
		{"advisory, degraded", model.EnforcementAdvisory, nil, true, model.OutcomeWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := gate.Evaluate(gate.Input{
				Scores:   score.Scores{Categories: map[model.Category]*int{}},
				Findings: tt.findings, Gate: violated, Enforcement: tt.enforcement, Degraded: tt.degraded,
			})
			if res.Outcome != tt.want || res.Degraded != tt.degraded {
				t.Errorf("outcome = %s (degraded %v), want %s", res.Outcome, res.Degraded, tt.want)
			}
		})
	}
}

func TestChecksAreListedInAStableOrder(t *testing.T) {
	t.Parallel()
	g := config.Gate{
		OverallMin:      h(1),
		CategoryMin:     map[model.Category]int{"z": 1, "a": 1, "m": 1},
		MaxNew:          map[model.Severity]int{model.SeverityLow: 1, model.SeverityCritical: 1, model.SeverityHigh: 1},
		RequireAssessed: []model.Category{"z", "a"},
	}
	in := gate.Input{Scores: score.Scores{Categories: map[model.Category]*int{}}, Gate: g}
	first := gate.Evaluate(in)
	for range 20 {
		again := gate.Evaluate(in)
		for i := range first.Checks {
			if first.Checks[i].Check != again.Checks[i].Check {
				t.Fatalf("order changed: %v vs %v", first.Checks, again.Checks)
			}
		}
	}
	var names []string
	for _, c := range first.Checks {
		names = append(names, c.Check)
	}
	want := []string{"overall_min", "category_min.a", "category_min.m", "category_min.z",
		"max_new.critical", "max_new.high", "max_new.low", "require_assessed.a", "require_assessed.z"}
	for i := range want {
		if i >= len(names) || names[i] != want[i] {
			t.Fatalf("order = %v, want %v", names, want)
		}
	}
}
