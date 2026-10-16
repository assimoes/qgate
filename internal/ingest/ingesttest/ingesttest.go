// Package ingesttest is the Seam C harness: it runs an ingestor over
// fixtures in testdata/ and compares the result with golden files.
package ingesttest

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/ingest"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Golden parses testdata/<fixture> for each fixture and compares the
// result with testdata/<fixture minus extension>.golden.json. Run the
// tests with -update to rewrite the goldens after checking a change.
func Golden(t *testing.T, ing ingest.Ingestor, fixtures ...string) {
	t.Helper()
	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join("testdata", fixture))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			name := fixture[:len(fixture)-len(filepath.Ext(fixture))]
			res, err := ing.Parse(context.Background(), f, config.Input{Name: name}, nil)
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
				t.Errorf("parse result differs from %s (run with -update after checking the change):\n%s", golden, got)
			}
		})
	}
}
