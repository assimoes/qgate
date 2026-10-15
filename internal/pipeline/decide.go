package pipeline

import (
	"fmt"
	"maps"
	"slices"

	"qgate/internal/config"
	"qgate/internal/gate"
	"qgate/internal/model"
	"qgate/internal/score"
)

// Decision is the deterministic part of a run: scores and the gate.
type Decision struct {
	Scores model.Scores
	Gate   model.GateResult
}

// Decide scores the findings and evaluates the gate. It is a pure function
// of its arguments, so `qgate run` and `qgate score --report` reach the
// same decision from the same findings, config and input statuses.
func Decide(findings []model.Finding, cfg config.Config, inputs []model.InputStatus, degraded bool) (Decision, error) {
	params := scoreParams(cfg.Scoring)
	scores, err := score.Score(findings, params, assessed(cfg.Inputs, inputs, findings))
	if err != nil {
		return Decision{}, fmt.Errorf("score: %w", err)
	}
	result := gate.Evaluate(gate.Input{
		Scores:      scores,
		Findings:    findings,
		Gate:        cfg.Profile.Gate,
		Enforcement: cfg.Profile.Enforcement,
		Scoring:     params,
		Degraded:    degraded,
	})

	out := model.Scores{Overall: score.Points(scores.Overall), Categories: map[string]*int{}}
	for c, v := range scores.Categories {
		out.Categories[string(c)] = score.Points(v)
	}
	return Decision{Scores: out, Gate: result}, nil
}

func scoreParams(s config.Scoring) score.Params {
	return score.Params{
		Version:            s.Version,
		SeverityWeights:    s.SeverityWeights,
		AgentWeightPct:     s.AgentWeightPct,
		AgentMinConfidence: s.AgentMinConfidence,
		AgentMaxPenalty:    s.AgentMaxPenalty,
		CategoryWeights:    s.CategoryWeights,
	}
}

// assessed returns the categories some successful input covered: each
// one's declared categories (category_default, else quality, plus
// assesses) and the categories of the findings it produced. A failed input
// assesses nothing, so a missing analyzer never yields a perfect score.
func assessed(cfgInputs []config.Input, statuses []model.InputStatus, findings []model.Finding) []model.Category {
	ok := map[string]bool{}
	for _, s := range statuses {
		if s.Status == model.InputOK {
			ok[s.Name] = true
		}
	}
	set := map[model.Category]bool{}
	for _, in := range cfgInputs {
		if !ok[in.Name] {
			continue
		}
		set[in.EffectiveCategory()] = true
		for _, c := range in.Assesses {
			set[c] = true
		}
	}
	for _, f := range findings {
		if f.Source.Kind == model.SourceTool && ok[f.Source.Name] {
			set[f.Category] = true
		}
	}
	return slices.Sorted(maps.Keys(set))
}

// Degraded reports whether any input failed.
func Degraded(statuses []model.InputStatus) bool {
	return slices.ContainsFunc(statuses, func(s model.InputStatus) bool { return s.Status == model.InputFailed })
}
