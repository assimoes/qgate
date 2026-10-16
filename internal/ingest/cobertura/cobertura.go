// Package cobertura ingests Cobertura XML coverage reports into coverage
// metrics (design §8.2).
package cobertura

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/ingest/coverage"
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

// Parse reads every class's lines.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, gc *gitctx.Context) (ingest.Result, error) {
	var rep report
	if err := xml.NewDecoder(r).Decode(&rep); err != nil {
		return ingest.Result{}, fmt.Errorf("decode cobertura: %w", err)
	}
	byPath := map[string]*coverage.File{}
	var res ingest.Result
	for _, pkg := range rep.Packages {
		for _, cls := range pkg.Classes {
			f := coverage.NewFile(cls.Filename)
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
					res.Warnings = append(res.Warnings, fmt.Sprintf("%s:%d: unreadable condition-coverage %q", cls.Filename, l.Number, l.ConditionCoverage))
					continue
				}
				covered, _ := strconv.Atoi(m[1])
				total, _ := strconv.Atoi(m[2])
				f.Branches[l.Number] = coverage.Branches{Covered: covered, Total: total}
			}
			if prev, ok := byPath[cls.Filename]; ok {
				prev.Merge(f)
			} else {
				byPath[cls.Filename] = &f
			}
		}
	}
	files := make([]coverage.File, 0, len(byPath))
	for _, p := range slices.Sorted(maps.Keys(byPath)) {
		files = append(files, *byPath[p])
	}
	res.Metrics = coverage.Metrics(files, gc)
	return res, nil
}
