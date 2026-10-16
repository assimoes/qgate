package lcov_test

import (
	"testing"

	"github.com/assimoes/qgate/internal/ingest/ingesttest"
	"github.com/assimoes/qgate/internal/ingest/lcov"
)

// The fixture follows Istanbul/nyc's lcov reporter (absolute paths,
// function and branch records, "-" for branches never reached). Replace it
// with captured output when the tool runs in CI; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, lcov.New(), "istanbul.info")
}
