package pipeline

import (
	"time"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/normalize"
)

// Excuse applies the excusal steps that span inputs, in design §9 order:
// suppressions (step 8), then tool dedup (step 9), so a suppressed finding
// never merges with a live one. The baseline (step 7, ticket 36) goes
// first. It returns the findings in canonical order and run-level
// warnings, sorted. now is the run's time.
func Excuse(findings []model.Finding, cfg config.Config, now time.Time) ([]model.Finding, []string) {
	findings, warnings := normalize.Suppress(findings, cfg.Suppressions, now)
	return normalize.DedupTools(findings), warnings
}
