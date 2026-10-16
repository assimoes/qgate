package report

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/assimoes/qgate/internal/model"
)

// consoleTopFindings is how many findings the console lists.
const consoleTopFindings = 10

// Console writes the human summary to a terminal or CI log: gate outcome
// first, then scores, failed gate checks and the top findings.
type Console struct {
	W io.Writer
	// Color adds ANSI colors, for a terminal that has not set NO_COLOR.
	Color bool
	// ReproducedFrom names the saved report `qgate score` recomputed this
	// result from; empty for a live run.
	ReproducedFrom string
}

// ANSI escape sequences.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

// Report writes the summary of r.
func (c Console) Report(_ context.Context, r *model.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Gate outcome: %s\n", c.paint(strings.ToUpper(string(r.Gate.Outcome)), outcomeColor(r.Gate.Outcome)+ansiBold))
	fmt.Fprintf(&b, "Stage: %s (%s, gating scope %s)\n", r.Run.Stage, r.Run.Enforcement, r.Run.GatingScope)
	if r.Gate.Degraded {
		b.WriteString(c.paint("Degraded run: an input or agent failed; see the report", ansiYellow) + "\n")
	}
	fmt.Fprintf(&b, "Overall score: %s\n", points(r.Scores.Overall))
	for _, cat := range slices.Sorted(maps.Keys(r.Scores.Categories)) {
		fmt.Fprintf(&b, "  %s: %s\n", cat, points(r.Scores.Categories[cat]))
	}
	for _, chk := range r.Gate.Checks {
		if chk.Result == model.CheckFail {
			fmt.Fprintf(&b, "%s %s (expected %s, actual %s)\n", c.paint("Failed:", ansiRed), chk.Check, chk.Expected, actual(chk.Actual))
		}
	}
	fmt.Fprintf(&b, "Findings: %d (%d new)\n", len(r.Findings), countNew(r.Findings))
	for _, f := range topFindings(r, consoleTopFindings) {
		sev := fmt.Sprintf("%-8s", strings.ToUpper(f.Severity.String()))
		fmt.Fprintf(&b, "  %s %s %s %s\n", c.paint(sev, severityColor(f.Severity)), f.RuleID, location(f), f.Title)
	}
	for _, w := range r.Warnings {
		fmt.Fprintf(&b, "%s %s\n", c.paint("Warning:", ansiYellow), w)
	}
	if c.ReproducedFrom != "" {
		fmt.Fprintf(&b, "Scores and gate reproduced exactly from %s\n", c.ReproducedFrom)
	}
	if _, err := io.WriteString(c.W, b.String()); err != nil {
		return fmt.Errorf("write console report: %w", err)
	}
	return nil
}

func (c Console) paint(s, color string) string {
	if !c.Color || color == "" {
		return s
	}
	return color + s + ansiReset
}

func outcomeColor(o model.Outcome) string {
	switch o {
	case model.OutcomePass:
		return ansiGreen
	case model.OutcomeWarn:
		return ansiYellow
	default:
		return ansiRed
	}
}

func severityColor(s model.Severity) string {
	switch {
	case s >= model.SeverityHigh:
		return ansiRed
	case s == model.SeverityMedium:
		return ansiYellow
	default:
		return ""
	}
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
