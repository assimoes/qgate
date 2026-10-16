// Package junit ingests JUnit XML test results into test metrics (design
// §8.2).
package junit

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
)

// Metric names.
const (
	TotalMetric   = "tests.total"
	FailedMetric  = "tests.failed"
	SkippedMetric = "tests.skipped"
)

// Ingestor parses JUnit XML.
type Ingestor struct{}

// New returns the JUnit ingestor.
func New() Ingestor { return Ingestor{} }

// Format is the format name inputs use.
func (Ingestor) Format() string { return "junit" }

// ProducesFindings is false: JUnit yields metrics only.
func (Ingestor) ProducesFindings() bool { return false }

// Parse counts <testcase> elements at any depth, whatever the root
// (<testsuites> or <testsuite>): a test with a <failure> or <error> child
// failed, one with <skipped> was skipped. Counting test cases rather than
// trusting suite attributes keeps nested and hand-merged files right.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, _ *gitctx.Context) (ingest.Result, error) {
	dec := xml.NewDecoder(r)
	var total, failed, skipped int
	inCase, caseFailed, caseSkipped := false, false, false
	sawRoot := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ingest.Result{}, fmt.Errorf("decode junit: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if !sawRoot {
				sawRoot = true
				if t.Name.Local != "testsuites" && t.Name.Local != "testsuite" {
					return ingest.Result{}, fmt.Errorf("decode junit: root element is <%s>, want <testsuites> or <testsuite>", t.Name.Local)
				}
			}
			switch {
			case t.Name.Local == "testcase":
				inCase, caseFailed, caseSkipped = true, false, false
			case inCase && (t.Name.Local == "failure" || t.Name.Local == "error"):
				caseFailed = true
			case inCase && t.Name.Local == "skipped":
				caseSkipped = true
			}
		case xml.EndElement:
			if t.Name.Local == "testcase" && inCase {
				total++
				switch {
				case caseFailed:
					failed++
				case caseSkipped:
					skipped++
				}
				inCase = false
			}
		}
	}
	if !sawRoot {
		return ingest.Result{}, errors.New("decode junit: empty document")
	}
	count := func(name string, n int) model.Metric {
		return model.Metric{Name: name, Value: float64(n), Unit: model.UnitCount, Scope: model.MetricScopeRepo}
	}
	return ingest.Result{Metrics: []model.Metric{
		count(TotalMetric, total), count(FailedMetric, failed), count(SkippedMetric, skipped),
	}}, nil
}
