package cobertura_test

import (
	"testing"

	"github.com/assimoes/qgate/internal/ingest/cobertura"
	"github.com/assimoes/qgate/internal/ingest/ingesttest"
)

// The fixtures are real gocover-cobertura and coverage.py output for
// testdata/capture/, captured by scripts/capture-fixtures.sh.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, cobertura.New(), "gocover.xml", "coveragepy.xml")
}
