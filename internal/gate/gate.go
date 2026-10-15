// Package gate evaluates a profile's gate checks (design §13). Evaluate is
// a pure function: the same scores, findings and gate always give the same
// result, which is what lets `qgate score --report` reproduce a run.
package gate

import (
	"fmt"
	"maps"
	"slices"

	"qgate/internal/config"
	"qgate/internal/model"
	"qgate/internal/score"
)

// Input is everything a gate decision depends on.
type Input struct {
	Scores             score.Scores // hundredths; nil means not assessed
	Findings           []model.Finding
	Gate               config.Gate
	Enforcement        model.Enforcement
	AgentMinConfidence int
	// Degraded says the run was incomplete: an input or agent failed, or
	// a budget or deadline was hit.
	Degraded bool
}

// Evaluate runs every gate check and decides the outcome.
func Evaluate(in Input) model.GateResult {
	g := in.Gate
	var checks []model.GateCheck

	if g.OverallMin != nil {
		checks = append(checks, minScore("overall_min", in.Scores.Overall, *g.OverallMin))
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
		return f.Source.Kind != model.SourceAgent || (f.Confidence != nil && *f.Confidence >= in.AgentMinConfidence)
	})...)
	checks = append(checks, maxNew("max_new_tool", g.MaxNewTool, in.Findings, func(f model.Finding) bool {
		return f.Source.Kind == model.SourceTool
	})...)
	for _, c := range slices.Sorted(slices.Values(g.RequireAssessed)) {
		assessed := in.Scores.Categories[c] != nil
		checks = append(checks, model.GateCheck{
			Check: "require_assessed." + string(c), Expected: "assessed", Actual: assessed, Result: result(assessed),
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
	c.Result = result(*hundredths >= minPoints*100)
	return c
}

// maxNew checks the count of new findings at exactly each severity.
func maxNew(prefix string, limits map[model.Severity]int, findings []model.Finding, counts func(model.Finding) bool) []model.GateCheck {
	var out []model.GateCheck
	for _, sev := range slices.Backward(slices.Sorted(maps.Keys(limits))) { // critical first
		n := 0
		for _, f := range findings {
			if f.Status == model.StatusNew && f.Severity == sev && counts(f) {
				n++
			}
		}
		out = append(out, model.GateCheck{
			Check: prefix + "." + sev.String(), Expected: fmt.Sprintf("<= %d", limits[sev]),
			Actual: n, Result: result(n <= limits[sev]),
		})
	}
	return out
}

func result(ok bool) model.CheckResult {
	if ok {
		return model.CheckPass
	}
	return model.CheckFail
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
