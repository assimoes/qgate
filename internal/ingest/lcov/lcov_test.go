package lcov_test

import (
	"testing"

	"github.com/assimoes/qgate/internal/ingest/ingesttest"
	"github.com/assimoes/qgate/internal/ingest/lcov"
)

// The fixture is real c8 (Istanbul lcov reporter) output for
// testdata/capture/, captured by scripts/capture-fixtures.sh.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, lcov.New(), "c8.info")
}
