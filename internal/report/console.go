package report

import (
	"context"
	"fmt"
	"io"
	"strings"

	"qgate/internal/model"
)

// Console writes the human summary to a terminal or CI log, gate outcome first.
type Console struct {
	W io.Writer
}

// Report writes the summary of r.
func (c Console) Report(_ context.Context, r *model.Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Gate outcome: %s\n", strings.ToUpper(string(r.Gate.Outcome)))
	fmt.Fprintf(&b, "Stage: %s (%s, gating scope %s)\n", r.Run.Stage, r.Run.Enforcement, r.Run.GatingScope)
	if r.Scores.Overall == nil {
		b.WriteString("Overall score: not assessed\n")
	} else {
		fmt.Fprintf(&b, "Overall score: %d\n", *r.Scores.Overall)
	}
	fmt.Fprintf(&b, "Findings: %d\n", len(r.Findings))
	if _, err := io.WriteString(c.W, b.String()); err != nil {
		return fmt.Errorf("write console report: %w", err)
	}
	return nil
}
