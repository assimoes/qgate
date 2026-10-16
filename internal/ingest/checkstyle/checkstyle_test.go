package checkstyle_test

import (
	"context"
	"strings"
	"testing"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/ingest/checkstyle"
	"github.com/assimoes/qgate/internal/ingest/ingesttest"
)

// The fixtures are real golangci-lint and ESLint output for
// testdata/capture/, captured by scripts/capture-fixtures.sh.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	ingesttest.Golden(t, checkstyle.New(), "golangci.xml", "eslint.xml")
}

func TestMalformedXMLIsAnError(t *testing.T) {
	t.Parallel()
	_, err := checkstyle.New().Parse(context.Background(), strings.NewReader("<checkstyle><file"), config.Input{}, nil)
	if err == nil {
		t.Error("want an error for malformed XML")
	}
}

func TestFileWithoutNameIsSkippedWithAWarning(t *testing.T) {
	t.Parallel()
	xml := `<checkstyle><file><error line="1" severity="error" source="a"/></file>` +
		`<file name="b.go"><error line="2" severity="error" source="b"/></file></checkstyle>`
	res, err := checkstyle.New().Parse(context.Background(), strings.NewReader(xml), config.Input{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 || res.Findings[0].Location.Path != "b.go" {
		t.Errorf("findings = %+v, want only b.go's", res.Findings)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "name") {
		t.Errorf("warnings = %v, want one about the missing file name", res.Warnings)
	}
}
