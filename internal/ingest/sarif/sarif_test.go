package sarif_test

import (
	"testing"

	"github.com/assimoes/qgate/internal/ingest/ingesttest"
	"github.com/assimoes/qgate/internal/ingest/sarif"
)

// The fixtures are real Semgrep, CodeQL and Trivy output for
// testdata/capture/, captured by scripts/capture-fixtures.sh.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, sarif.New(), "semgrep.sarif", "codeql.sarif", "trivy.sarif")
}
