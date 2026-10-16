package score_test

import (
	"encoding/json"
	"flag"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/score"
)

// referenceParams are the scoring parameters of the design's reference
// config (design §5.3).
func referenceParams() score.Params {
	return score.Params{
		Version: 1,
		SeverityWeights: map[model.Severity]int{
			model.SeverityCritical: 40, model.SeverityHigh: 20, model.SeverityMedium: 8,
			model.SeverityLow: 2, model.SeverityInfo: 0,
		},
		AgentWeightPct:     50,
		AgentMinConfidence: 70,
		AgentMaxPenalty:    15,
		CategoryWeights: map[model.Category]int{
			"security": 40, "quality": 30, "maintainability": 20, "testing": 10,
		},
	}
}

func confidence(c int) *int { return &c }

func toolFinding(cat model.Category, sev model.Severity) model.Finding {
	return model.Finding{Category: cat, Severity: sev, Status: model.StatusNew,
		Source: model.Source{Kind: model.SourceTool, Name: "semgrep"}}
}

func agentFinding(cat model.Category, sev model.Severity, conf int) model.Finding {
	return model.Finding{Category: cat, Severity: sev, Status: model.StatusNew, Confidence: confidence(conf),
		Source: model.Source{Kind: model.SourceAgent, Name: "security-reviewer"}}
}

func hundredths(t *testing.T, p *int) int {
	t.Helper()
	if p == nil {
		t.Fatal("score is null, want a value")
	}
	return *p
}

var update = flag.Bool("update", false, "rewrite golden files")

// TestWorkedExample is the golden example of design §12.3: one new high
// tool finding and one new medium agent finding at confidence 85 in
// security score 7600 hundredths, reported as 76.
func TestWorkedExample(t *testing.T) {
	t.Parallel()
	findings := []model.Finding{
		toolFinding("security", model.SeverityHigh),
		agentFinding("security", model.SeverityMedium, 85),
	}
	s, err := score.Score(findings, referenceParams(), []model.Category{"security"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(struct {
		Hundredths score.Scores `json:"hundredths"`
		Reported   *int         `json:"reported_security"`
	}{s, score.Points(s.Categories["security"])}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	golden := filepath.Join("testdata", "worked_example.golden.json")
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("worked example differs from %s:\n%s", golden, got)
	}
}

func TestOverallIsWeightedOverAssessedCategories(t *testing.T) {
	t.Parallel()
	findings := []model.Finding{
		toolFinding("security", model.SeverityHigh),    // security 8000
		toolFinding("quality", model.SeverityCritical), // quality 6000
		toolFinding("testing", model.SeverityCritical), // testing not assessed: ignored
	}
	s, err := score.Score(findings, referenceParams(), []model.Category{"security", "quality", "maintainability"})
	if err != nil {
		t.Fatal(err)
	}
	// (40×8000 + 30×6000 + 20×10000) / 90 = 700000/90 = 7777 (integer division)
	if got := hundredths(t, s.Overall); got != 7777 {
		t.Errorf("overall = %d, want 7777", got)
	}
	if s.Categories["testing"] != nil {
		t.Errorf("testing = %d, want null (not assessed)", *s.Categories["testing"])
	}
	if got := score.Points(s.Overall); *got != 77 {
		t.Errorf("reported overall = %d, want 77 (floored)", *got)
	}
}

func TestNothingAssessedMeansNullOverall(t *testing.T) {
	t.Parallel()
	s, err := score.Score([]model.Finding{toolFinding("security", model.SeverityLow)}, referenceParams(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Overall != nil {
		t.Errorf("overall = %d, want null", *s.Overall)
	}
}

func TestScoreFloorsAtZero(t *testing.T) {
	t.Parallel()
	var fs []model.Finding
	for range 5 {
		fs = append(fs, toolFinding("security", model.SeverityCritical))
	}
	s, _ := score.Score(fs, referenceParams(), []model.Category{"security"})
	if got := hundredths(t, s.Categories["security"]); got != 0 {
		t.Errorf("security = %d, want 0", got)
	}
}

func TestAgentFindingsBelowMinConfidenceNeverScore(t *testing.T) {
	t.Parallel()
	s, _ := score.Score([]model.Finding{agentFinding("security", model.SeverityCritical, 69)},
		referenceParams(), []model.Category{"security"})
	if got := hundredths(t, s.Categories["security"]); got != 10000 {
		t.Errorf("security = %d, want 10000", got)
	}
}

func TestUnknownAlgorithmVersionIsAnError(t *testing.T) {
	t.Parallel()
	p := referenceParams()
	p.Version = 2
	if _, err := score.Score(nil, p, nil); err == nil {
		t.Error("want an error for scoring version 2")
	}
}

// randomFindings draws findings across categories, severities, sources,
// statuses and confidences.
func randomFindings(r *rand.Rand, n int) []model.Finding {
	cats := []model.Category{"security", "quality", "maintainability", "testing"}
	statuses := []model.Status{model.StatusNew, model.StatusNew, model.StatusOutOfScope,
		model.StatusBaseline, model.StatusSuppressed, model.StatusSuperseded, model.StatusInvalid}
	fs := make([]model.Finding, n)
	for i := range fs {
		sev := model.Severity(r.IntN(5))
		cat := cats[r.IntN(len(cats))]
		if r.IntN(2) == 0 {
			fs[i] = toolFinding(cat, sev)
		} else {
			fs[i] = agentFinding(cat, sev, r.IntN(101))
		}
		fs[i].Status = statuses[r.IntN(len(statuses))]
	}
	return fs
}

var allCategories = []model.Category{"security", "quality", "maintainability", "testing"}

func mustScore(t *testing.T, fs []model.Finding) score.Scores {
	t.Helper()
	s, err := score.Score(fs, referenceParams(), allCategories)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func equalScores(a, b score.Scores) bool {
	eq := func(x, y *int) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	if !eq(a.Overall, b.Overall) || len(a.Categories) != len(b.Categories) {
		return false
	}
	for c, v := range a.Categories {
		if !eq(v, b.Categories[c]) {
			return false
		}
	}
	return true
}

func TestPropertyOrderIndependence(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		fs := randomFindings(r, r.IntN(30))
		want := mustScore(t, fs)
		shuffled := slices.Clone(fs)
		r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got := mustScore(t, shuffled); !equalScores(got, want) {
			t.Fatalf("scores changed with order:\n%v\n%v", want, got)
		}
	}
}

func TestPropertyAddingANewFindingNeverRaisesAScore(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(3, 4))
	for range 200 {
		fs := randomFindings(r, r.IntN(30))
		before := mustScore(t, fs)
		extra := randomFindings(r, 1)[0]
		extra.Status = model.StatusNew
		after := mustScore(t, append(slices.Clone(fs), extra))
		for _, c := range allCategories {
			if *after.Categories[c] > *before.Categories[c] {
				t.Fatalf("%s rose from %d to %d after adding %+v", c, *before.Categories[c], *after.Categories[c], extra)
			}
		}
		if *after.Overall > *before.Overall {
			t.Fatalf("overall rose from %d to %d", *before.Overall, *after.Overall)
		}
	}
}

func TestPropertyExcusedFindingsNeverChangeScores(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(5, 6))
	excused := []model.Status{model.StatusOutOfScope, model.StatusBaseline, model.StatusSuppressed,
		model.StatusSuperseded, model.StatusInvalid}
	for range 200 {
		fs := randomFindings(r, r.IntN(30))
		before := mustScore(t, fs)
		extra := randomFindings(r, 1)[0]
		extra.Status = excused[r.IntN(len(excused))]
		if after := mustScore(t, append(slices.Clone(fs), extra)); !equalScores(after, before) {
			t.Fatalf("a %s finding changed the scores", extra.Status)
		}
	}
}

func TestPropertyAgentPenaltyIsCapped(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(7, 8))
	p := referenceParams()
	floor := 10000 - p.AgentMaxPenalty*100
	for range 200 {
		var fs []model.Finding
		for range r.IntN(50) {
			fs = append(fs, agentFinding("security", model.Severity(r.IntN(5)), r.IntN(101)))
		}
		s := mustScore(t, fs)
		if got := *s.Categories["security"]; got < floor {
			t.Fatalf("agents alone took security to %d, below the cap floor %d", got, floor)
		}
	}
}
