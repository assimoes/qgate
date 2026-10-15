// Package score is the deterministic scoring engine (design §12). Scores
// are pure functions of new findings: no I/O, clock or randomness, integer
// arithmetic in hundredths of a point. Each algorithm version is frozen once
// released; a change is a new version, never an edit (see ADR-0001).
package score

import (
	"fmt"
	"maps"
	"slices"

	"qgate/internal/model"
)

// AlgorithmVersion is the latest scoring algorithm this build implements.
const AlgorithmVersion = 1

// full is a perfect score, in hundredths.
const full = 10000

// Params are a scoring version and its parameters (scoring.* in config).
type Params struct {
	Version            int
	SeverityWeights    map[model.Severity]int
	AgentWeightPct     int
	AgentMinConfidence int
	AgentMaxPenalty    int // points per category
	CategoryWeights    map[model.Category]int
}

// Scores are in hundredths of a point; nil means not assessed.
type Scores struct {
	Overall    *int
	Categories map[model.Category]*int // every assessed category, plus not-assessed ones scored nil
}

// algorithm computes category scores for the assessed categories.
type algorithm func(findings []model.Finding, p Params, assessed []model.Category) map[model.Category]int

// algorithms are the released scoring versions.
var algorithms = map[int]algorithm{
	1: scoreV1,
}

// Score scores findings with the algorithm version p selects. Categories
// in assessed are scored; every other category is null and excluded from
// the overall score.
func Score(findings []model.Finding, p Params, assessed []model.Category) (Scores, error) {
	algo, ok := algorithms[p.Version]
	if !ok {
		return Scores{}, fmt.Errorf("scoring version %d is not supported (this build supports %v)",
			p.Version, slices.Sorted(maps.Keys(algorithms)))
	}
	categories := algo(findings, p, assessed)

	s := Scores{Categories: map[model.Category]*int{}}
	for c := range p.CategoryWeights {
		s.Categories[c] = nil
	}
	weighted, weights := 0, 0
	for _, c := range slices.Sorted(maps.Keys(categories)) {
		v := categories[c]
		s.Categories[c] = &v
		weighted += p.CategoryWeights[c] * v
		weights += p.CategoryWeights[c]
	}
	if weights > 0 {
		overall := weighted / weights
		s.Overall = &overall
	}
	return s, nil
}

// scoreV1 is design §12.2. Frozen.
func scoreV1(findings []model.Finding, p Params, assessed []model.Category) map[model.Category]int {
	toolPenalty := map[model.Category]int{}
	agentPenalty := map[model.Category]int{}
	for _, f := range findings {
		if f.Status != model.StatusNew {
			continue
		}
		w := p.SeverityWeights[f.Severity]
		switch f.Source.Kind {
		case model.SourceAgent:
			if f.Confidence != nil && *f.Confidence >= p.AgentMinConfidence {
				agentPenalty[f.Category] += w * p.AgentWeightPct
			}
		default:
			toolPenalty[f.Category] += w * 100
		}
	}
	out := make(map[model.Category]int, len(assessed))
	for _, c := range assessed {
		agent := min(p.AgentMaxPenalty*100, agentPenalty[c])
		out[c] = max(0, full-toolPenalty[c]-agent)
	}
	return out
}

// Points converts hundredths to the whole points reports show, flooring
// (76.99 → 76). Nil stays nil.
func Points(hundredths *int) *int {
	if hundredths == nil {
		return nil
	}
	p := *hundredths / 100
	return &p
}
