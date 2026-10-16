package cobertura_test

import (
	"testing"

	"qgate/internal/ingest/cobertura"
	"qgate/internal/ingest/ingesttest"
)

// Fixtures follow gocover-cobertura (Go import paths, no branch data) and
// coverage.py (branch condition coverage). Replace them with captured
// output when the tools run in CI; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, cobertura.New(), "gocover.xml", "coveragepy.xml")
}
