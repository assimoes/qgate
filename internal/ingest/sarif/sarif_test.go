package sarif_test

import (
	"testing"

	"github.com/assimoes/qgate/internal/ingest/ingesttest"
	"github.com/assimoes/qgate/internal/ingest/sarif"
)

// The fixtures follow each analyzer's SARIF output shape (Semgrep OSS,
// CodeQL with rules in query-pack extensions, Trivy). Replace them with
// captured output when the analyzers are available; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, sarif.New(), "semgrep.sarif", "codeql.sarif", "trivy.sarif")
}
