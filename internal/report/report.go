// Package report renders a run's report. Each output format is a Reporter;
// adding a format means adding a Reporter, not editing existing ones.
package report

import (
	"context"

	"qgate/internal/model"
)

// Reporter renders a report to one destination.
type Reporter interface {
	Report(ctx context.Context, r *model.Report) error
}
