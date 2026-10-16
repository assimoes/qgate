// Package checkstyle ingests Checkstyle XML, as written by golangci-lint,
// ESLint's checkstyle formatter and many others (design §8.2).
package checkstyle

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/gitctx"
	"github.com/assimoes/qgate/internal/ingest"
	"github.com/assimoes/qgate/internal/model"
)

// unnamedRule is the rule ID for errors without a source, such as ESLint's
// fatal parse errors.
const unnamedRule = "unnamed"

// Ingestor parses Checkstyle XML.
type Ingestor struct{}

// New returns the Checkstyle ingestor.
func New() Ingestor { return Ingestor{} }

// Format is the format name inputs use.
func (Ingestor) Format() string { return "checkstyle" }

// ProducesFindings is true: Checkstyle reports findings.
func (Ingestor) ProducesFindings() bool { return true }

type report struct {
	Files []file `xml:"file"`
}

type file struct {
	Name   string     `xml:"name,attr"`
	Errors []errorTag `xml:"error"`
}

type errorTag struct {
	Line     string `xml:"line,attr"`
	Column   string `xml:"column,attr"`
	Severity string `xml:"severity,attr"`
	Message  string `xml:"message,attr"`
	Source   string `xml:"source,attr"`
}

// severity is the builtin mapping for Checkstyle severity labels.
func severity(label string) (model.Severity, bool) {
	switch label {
	case "error":
		return model.SeverityHigh, true
	case "warning":
		return model.SeverityMedium, true
	case "info":
		return model.SeverityLow, true
	case "ignore":
		return model.SeverityInfo, true
	default:
		return 0, false
	}
}

// Parse reads every <error> of every <file>.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, _ *gitctx.Context) (ingest.Result, error) {
	var rep report
	if err := xml.NewDecoder(r).Decode(&rep); err != nil {
		return ingest.Result{}, fmt.Errorf("decode checkstyle: %w", err)
	}
	var res ingest.Result
	unnamed, nameless := 0, 0
	for _, fl := range rep.Files {
		if fl.Name == "" {
			nameless += len(fl.Errors)
			continue
		}
		for _, e := range fl.Errors {
			f := ingest.Finding{}
			f.RuleID = e.Source
			if f.RuleID == "" {
				f.RuleID = unnamedRule
				unnamed++
			}
			f.Title = f.RuleID
			f.Message = e.Message
			f.Location.Path = fl.Name
			f.Location.StartLine = intAttr(e.Line)
			f.Location.EndLine = f.Location.StartLine
			f.Location.StartCol = intAttr(e.Column)
			f.Source = model.Source{Kind: model.SourceTool}
			f.RawSeverity = e.Severity
			if s, ok := severity(e.Severity); ok {
				f.DefaultSeverity = ingest.SeverityPtr(s)
			}
			res.Findings = append(res.Findings, f)
		}
	}
	if nameless > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("skipped errors in <file> elements without a name: %d", nameless))
	}
	if unnamed > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("rule ID %q given to errors without a source attribute: %d", unnamedRule, unnamed))
	}
	return res, nil
}

// intAttr reads a non-negative integer attribute. A missing or invalid
// value is 0, which means file-level for a line.
func intAttr(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
