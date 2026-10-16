// Package score is the deterministic scoring engine (design §12). Scores
// are pure functions of new findings: no I/O, clock or randomness, integer
// arithmetic in hundredths of a point. Each algorithm version is frozen once
// released; a change is a new version, never an edit (see ADR-0001).
package score

import (
	"fmt"
	"maps"
	"slices"

	"github.com/assimoes/qgate/internal/model"
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

// algorithm is one frozen scoring version: category and overall scores for
// the assessed categories.
type algorithm func(findings []model.Finding, p Params, assessed []model.Category) Scores

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
	return algo(findings, p, assessed), nil
}

// Counted reports whether a new finding counts against scores and gate
// checks: every tool finding, and agent findings at or above the minimum
// confidence. Released algorithms depend on it, so changing it changes them.
func Counted(f model.Finding, agentMinConfidence int) bool {
	if f.Source.Kind != model.SourceAgent {
		return true
	}
	return f.Confidence != nil && *f.Confidence >= agentMinConfidence
}

// scoreV1 is design §12.2. Frozen: a change is a new version.
func scoreV1(findings []model.Finding, p Params, assessed []model.Category) Scores {
	toolPenalty := map[model.Category]int{}
	agentPenalty := map[model.Category]int{}
	for _, f := range findings {
		if f.Status != model.StatusNew || !Counted(f, p.AgentMinConfidence) {
			continue
		}
		w := p.SeverityWeights[f.Severity]
		if f.Source.Kind == model.SourceAgent {
			agentPenalty[f.Category] += w * p.AgentWeightPct
		} else {
			toolPenalty[f.Category] += w * 100
		}
	}

	s := Scores{Categories: map[model.Category]*int{}}
	for c := range p.CategoryWeights {
		s.Categories[c] = nil // not assessed unless set below
	}
	weighted, weights := 0, 0
	for _, c := range slices.Sorted(slices.Values(assessed)) {
		v := max(0, full-toolPenalty[c]-min(p.AgentMaxPenalty*100, agentPenalty[c]))
		s.Categories[c] = &v
		weighted += p.CategoryWeights[c] * v
		weights += p.CategoryWeights[c]
	}
	if weights > 0 {
		overall := weighted / weights
		s.Overall = &overall
	}
	return s
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
