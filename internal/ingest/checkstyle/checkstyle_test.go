package checkstyle_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"qgate/internal/config"
	"qgate/internal/ingest/checkstyle"
)

var update = flag.Bool("update", false, "rewrite golden files")

// The fixtures follow golangci-lint's and ESLint's checkstyle output (ESLint
// writes absolute paths and an empty source for parse errors). Replace them
// with captured output when the linters are available; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"golangci", "eslint"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join("testdata", name+".xml"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			res, err := checkstyle.New().Parse(context.Background(), f, config.Input{Name: name}, nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			golden := filepath.Join("testdata", name+".golden.json")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create it): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("parse result differs from %s:\n%s", golden, got)
			}
		})
	}
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
