package report

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"qgate/internal/model"
)

// Console writes the human summary to a terminal or CI log, gate outcome first.
type Console struct {
	W io.Writer
	// ReproducedFrom names the saved report `qgate score` recomputed this
	// result from; empty for a live run.
	ReproducedFrom string
}

// Report writes the summary of r.
func (c Console) Report(_ context.Context, r *model.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Gate outcome: %s\n", strings.ToUpper(string(r.Gate.Outcome)))
	fmt.Fprintf(&b, "Stage: %s (%s, gating scope %s)\n", r.Run.Stage, r.Run.Enforcement, r.Run.GatingScope)
	if r.Gate.Degraded {
		b.WriteString("Degraded run: an input or agent failed; see the report\n")
	}
	fmt.Fprintf(&b, "Overall score: %s\n", points(r.Scores.Overall))
	for _, c := range slices.Sorted(maps.Keys(r.Scores.Categories)) {
		fmt.Fprintf(&b, "  %s: %s\n", c, points(r.Scores.Categories[c]))
	}
	for _, c := range r.Gate.Checks {
		if c.Result == model.CheckFail {
			fmt.Fprintf(&b, "Failed: %s (expected %s, actual %s)\n", c.Check, c.Expected, actual(c.Actual))
		}
	}
	fmt.Fprintf(&b, "Findings: %d (%d new)\n", len(r.Findings), countNew(r.Findings))
	if c.ReproducedFrom != "" {
		fmt.Fprintf(&b, "Scores and gate reproduced exactly from %s\n", c.ReproducedFrom)
	}
	if _, err := io.WriteString(c.W, b.String()); err != nil {
		return fmt.Errorf("write console report: %w", err)
	}
	return nil
}

func points(p *int) string {
	if p == nil {
		return "not assessed"
	}
	return strconv.Itoa(*p)
}

func actual(v any) string {
	if v == nil {
		return "not assessed"
	}
	return fmt.Sprint(v)
}

func countNew(fs []model.Finding) int {
	n := 0
	for _, f := range fs {
		if f.Status == model.StatusNew {
			n++
		}
	}
	return n
}
