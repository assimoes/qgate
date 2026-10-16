// Package cobertura ingests Cobertura XML coverage reports into coverage
// metrics (design §8.2).
package cobertura

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/gitctx"
	"github.com/assimoes/qgate/internal/ingest"
	"github.com/assimoes/qgate/internal/ingest/coverage"
)

// Ingestor parses Cobertura XML.
type Ingestor struct{}

// New returns the Cobertura ingestor.
func New() Ingestor { return Ingestor{} }

// Format is the format name inputs use.
func (Ingestor) Format() string { return "cobertura" }

// ProducesFindings is false: Cobertura yields metrics only.
func (Ingestor) ProducesFindings() bool { return false }

type report struct {
	Sources  []string `xml:"sources>source"`
	Packages []struct {
		Classes []struct {
			Filename string `xml:"filename,attr"`
			Lines    []line `xml:"lines>line"`
		} `xml:"classes>class"`
	} `xml:"packages>package"`
}

type line struct {
	Number            int    `xml:"number,attr"`
	Hits              int    `xml:"hits,attr"`
	Branch            bool   `xml:"branch,attr"`
	ConditionCoverage string `xml:"condition-coverage,attr"`
}

// conditionCoverage reads "50% (1/2)".
var conditionCoverage = regexp.MustCompile(`\((\d+)/(\d+)\)`)

// Parse reads every class's lines. Filenames are relative to the report's
// <source> directories, so each joined path is a candidate, then the bare
// filename.
func (Ingestor) Parse(ctx context.Context, r io.Reader, _ config.Input, gc *gitctx.Context) (ingest.Result, error) {
	var rep report
	if err := xml.NewDecoder(r).Decode(&rep); err != nil {
		return ingest.Result{}, fmt.Errorf("decode cobertura: %w", err)
	}
	files := coverage.Files{}
	var warnings []string
	for _, pkg := range rep.Packages {
		for _, cls := range pkg.Classes {
			f := coverage.NewFile(candidates(rep.Sources, cls.Filename)...)
			for _, l := range cls.Lines {
				if l.Number <= 0 {
					continue
				}
				f.Lines[l.Number] = l.Hits
				if !l.Branch {
					continue
				}
				m := conditionCoverage.FindStringSubmatch(l.ConditionCoverage)
				if m == nil {
					warnings = append(warnings, fmt.Sprintf("%s:%d: unreadable condition-coverage %q", cls.Filename, l.Number, l.ConditionCoverage))
					continue
				}
				covered, _ := strconv.Atoi(m[1])
				total, _ := strconv.Atoi(m[2])
				f.Branches[l.Number] = coverage.Branches{Covered: covered, Total: total}
			}
			files.Add(f)
		}
	}
	return files.Result(ctx, gc, warnings)
}

func candidates(sources []string, filename string) []string {
	var out []string
	for _, s := range sources {
		if s = strings.TrimSpace(s); s != "" && !path.IsAbs(filename) {
			out = append(out, path.Join(strings.ReplaceAll(s, `\`, "/"), filename))
		}
	}
	return append(out, filename)
}
