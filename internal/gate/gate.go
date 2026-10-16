// Package gate evaluates a profile's gate checks (design §13). Evaluate is
// a pure function: the same scores, findings and gate always give the same
// result, which is what lets `qgate score --report` reproduce a run.
package gate

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/score"
)

// Input is everything a gate decision depends on.
type Input struct {
	Scores      score.Scores // hundredths; nil means not assessed
	Findings    []model.Finding
	Metrics     []model.Metric // in canonical order
	Gate        config.Gate
	Enforcement model.Enforcement
	Scoring     score.Params
	// Degraded says the run was incomplete: an input or agent failed, or
	// a budget or deadline was hit.
	Degraded bool
}

// Evaluate runs every gate check and decides the outcome.
func Evaluate(in Input) model.GateResult {
	g := in.Gate
	var checks []model.GateCheck

	if g.OverallMin != nil {
		check := minScore("overall_min", in.Scores.Overall, *g.OverallMin)
		if in.Scores.Overall == nil && anyAssessed(in.Scores) {
			check.Note = "assessed categories carry no weight in scoring.category_weights, so there is no overall score"
		}
		checks = append(checks, check)
	}
	required := map[model.Category]bool{}
	for _, c := range g.RequireAssessed {
		required[c] = true
	}
	for _, c := range slices.Sorted(maps.Keys(g.CategoryMin)) {
		check := minScore("category_min."+string(c), in.Scores.Categories[c], g.CategoryMin[c])
		if in.Scores.Categories[c] == nil {
			check.Note = fmt.Sprintf("category %s was not assessed", c)
			if !required[c] {
				check.Result = model.CheckSkipped
				check.Note += "; skipped because it is not in require_assessed"
			}
		}
		checks = append(checks, check)
	}
	checks = append(checks, maxNew("max_new", g.MaxNew, in.Findings, func(f model.Finding) bool {
		return score.Counted(f, in.Scoring.AgentMinConfidence)
	})...)
	checks = append(checks, maxNew("max_new_tool", g.MaxNewTool, in.Findings, func(f model.Finding) bool {
		return f.Source.Kind == model.SourceTool
	})...)
	checks = append(checks, metricChecks(g.Metrics, in.Metrics)...)
	for _, c := range slices.Sorted(slices.Values(g.RequireAssessed)) {
		assessed := in.Scores.Categories[c] != nil
		checks = append(checks, model.GateCheck{
			Check: "require_assessed." + string(c), Expected: "assessed", Actual: assessed, Result: checkResult(assessed),
		})
	}

	violated := false
	for _, c := range checks {
		if c.Result == model.CheckFail {
			violated = true
		}
	}
	if checks == nil {
		checks = []model.GateCheck{}
	}
	return model.GateResult{Outcome: outcome(in.Enforcement, violated, in.Degraded), Degraded: in.Degraded, Checks: checks}
}

// minScore checks a score in hundredths against a minimum in points. A
// null score fails; callers decide whether that becomes a skip.
func minScore(name string, hundredths *int, minPoints int) model.GateCheck {
	c := model.GateCheck{Check: name, Expected: fmt.Sprintf(">= %d", minPoints)}
	if hundredths == nil {
		c.Result = model.CheckFail
		return c
	}
	c.Actual = *score.Points(hundredths)
	c.Result = checkResult(*hundredths >= minPoints*100)
	return c
}

// maxNew checks the count of new findings at exactly each severity, among
// those eligible counts.
func maxNew(prefix string, limits map[model.Severity]int, findings []model.Finding, eligible func(model.Finding) bool) []model.GateCheck {
	var out []model.GateCheck
	for _, sev := range slices.Backward(slices.Sorted(maps.Keys(limits))) { // critical first
		n := 0
		for _, f := range findings {
			if f.Status == model.StatusNew && f.Severity == sev && eligible(f) {
				n++
			}
		}
		out = append(out, model.GateCheck{
			Check: prefix + "." + sev.String(), Expected: fmt.Sprintf("<= %d", limits[sev]),
			Actual: n, Result: checkResult(n <= limits[sev]),
		})
	}
	return out
}

// defaultMetricScope applies to metric gate checks that name no scope.
const defaultMetricScope = model.MetricScopeRepo

// metricChecks evaluates metric gate checks in config order. Each is named
// metrics.<name>@<scope>, with #2, #3… when the same metric is checked more
// than once. When several inputs report the same metric at the same scope,
// the first in canonical order (alphabetical input name) is used.
func metricChecks(checks []config.MetricCheck, metrics []model.Metric) []model.GateCheck {
	var out []model.GateCheck
	seen := map[string]int{}
	for _, mc := range checks {
		scope := mc.Scope
		if scope == "" {
			scope = defaultMetricScope
		}
		name := "metrics." + mc.Name + "@" + string(scope)
		seen[name]++
		if n := seen[name]; n > 1 {
			name += "#" + strconv.Itoa(n)
		}
		c := model.GateCheck{Check: name, Expected: mc.Op + " " + formatNumber(mc.Value)}
		i := slices.IndexFunc(metrics, func(m model.Metric) bool { return m.Name == mc.Name && m.Scope == scope })
		switch {
		case i < 0 && mc.Optional:
			c.Result, c.Note = model.CheckSkipped, "metric not reported; the check is optional"
		case i < 0:
			c.Result, c.Note = model.CheckFail, "metric not reported by any input"
		default:
			c.Actual = metrics[i].Value
			c.Result = checkResult(compare(metrics[i].Value, mc.Op, mc.Value))
		}
		out = append(out, c)
	}
	return out
}

// compare applies a metric check operator; the config schema allows only
// these six.
func compare(actual float64, op string, want float64) bool {
	switch op {
	case ">=":
		return actual >= want
	case ">":
		return actual > want
	case "<=":
		return actual <= want
	case "<":
		return actual < want
	case "==":
		return actual == want
	case "!=":
		return actual != want
	default: // the schema admits no other operator
		return false
	}
}

func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func checkResult(ok bool) model.CheckResult {
	if ok {
		return model.CheckPass
	}
	return model.CheckFail
}

func anyAssessed(s score.Scores) bool {
	for _, v := range s.Categories {
		if v != nil {
			return true
		}
	}
	return false
}

// outcome applies design §13: advisory never fails; blocking fails on any
// violation; a degraded run is at best a warning.
func outcome(e model.Enforcement, violated, degraded bool) model.Outcome {
	switch {
	case e == model.EnforcementBlocking && violated:
		return model.OutcomeFail
	case violated || degraded:
		return model.OutcomeWarn
	default:
		return model.OutcomePass
	}
}
