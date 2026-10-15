package sarif_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"qgate/internal/config"
	"qgate/internal/ingest/sarif"
)

var update = flag.Bool("update", false, "rewrite golden files")

// The fixtures follow each analyzer's SARIF output shape (Semgrep OSS,
// CodeQL with rules in query-pack extensions, Trivy). Replace them with
// captured output when the analyzers are available; see docs/DECISIONS.md.
func TestParseGolden(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"semgrep", "codeql", "trivy"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join("testdata", name+".sarif"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			res, err := sarif.New().Parse(context.Background(), f, config.Input{Name: name}, nil)
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
				t.Errorf("parse result differs from %s (run with -update after checking the change):\ngot:\n%s", golden, got)
			}
		})
	}
}
