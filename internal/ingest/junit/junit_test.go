package junit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/ingest/ingesttest"
	"github.com/assimoes/qgate/internal/ingest/junit"
)

// The fixtures are real go-junit-report and pytest output for
// testdata/capture/, captured by scripts/capture-fixtures.sh.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, junit.New(), "gojunitreport.xml", "pytest.xml")
}

func TestNotJUnit(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{"", "<coverage/>", "<testsuites><testcase>"} {
		if _, err := junit.New().Parse(context.Background(), strings.NewReader(doc), config.Input{}, nil); err == nil {
			t.Errorf("%q: want an error", doc)
		}
	}
}
