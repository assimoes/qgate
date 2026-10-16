package pipeline

import (
	"fmt"
	"maps"
	"slices"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/gate"
	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/score"
)

// Decision is the deterministic part of a run: scores and the gate.
type Decision struct {
	Scores model.Scores
	Gate   model.GateResult
}

// Evidence is what a run observed: everything a decision depends on
// besides the config. A saved report carries all of it.
type Evidence struct {
	Findings []model.Finding
	Metrics  []model.Metric
	Inputs   []model.InputStatus
	// Degraded says the run was incomplete (an input or agent failed, or a
	// budget or deadline was hit).
	Degraded bool
}

// Decide scores the findings and evaluates the gate. It is a pure function
// of the evidence and config, both of which a report records, so `qgate
// run` and `qgate score --report` reach the same decision whatever formats
// the re-scoring build knows.
func Decide(ev Evidence, cfg config.Config) (Decision, error) {
	params := scoreParams(cfg.Scoring)
	scores, err := score.Score(ev.Findings, params, assessed(cfg.Inputs, ev.Inputs, ev.Findings))
	if err != nil {
		return Decision{}, fmt.Errorf("score: %w", err)
	}
	result := gate.Evaluate(gate.Input{
		Scores:      scores,
		Findings:    ev.Findings,
		Metrics:     ev.Metrics,
		Gate:        cfg.Profile.Gate,
		Enforcement: cfg.Profile.Enforcement,
		Scoring:     params,
		Degraded:    ev.Degraded,
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

// assessed returns the categories some successful input covered: what it
// declares (category_default and assesses; a findings input without
// category_default assesses the default category, where its findings go)
// and the categories of the findings it produced. A metrics-only input
// assesses only what it declares. A failed input assesses nothing, so a
// missing analyzer never yields a perfect score.
func assessed(cfgInputs []config.Input, statuses []model.InputStatus, findings []model.Finding) []model.Category {
	ok := map[string]bool{}
	findingsInput := map[string]bool{}
	for _, s := range statuses {
		if s.Status == model.InputOK {
			ok[s.Name] = true
		}
		findingsInput[s.Name] = s.Kind != model.InputKindMetrics
	}
	set := map[model.Category]bool{}
	for _, in := range cfgInputs {
		if !ok[in.Name] {
			continue
		}
		if in.CategoryDefault != "" || findingsInput[in.Name] {
			set[in.EffectiveCategory()] = true
		}
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
