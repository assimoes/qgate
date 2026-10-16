package junit_test

import (
	"context"
	"strings"
	"testing"

	"qgate/internal/config"
	"qgate/internal/ingest/ingesttest"
	"qgate/internal/ingest/junit"
)

// Fixtures follow go-junit-report (subtests as test cases, skipped
// messages) and pytest (single-line XML, setup errors as <error>). Replace
// them with captured output when the tools run in CI; see docs/DECISIONS.md.
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
