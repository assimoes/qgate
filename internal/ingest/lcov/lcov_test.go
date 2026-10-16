package lcov_test

import (
	"testing"

	"qgate/internal/ingest/ingesttest"
	"qgate/internal/ingest/lcov"
)

// The fixture follows Istanbul/nyc's lcov reporter (absolute paths,
// function and branch records, "-" for branches never reached). Replace it
// with captured output when the tool runs in CI; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, lcov.New(), "istanbul.info")
}
