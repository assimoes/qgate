// Package checkstyle ingests Checkstyle XML, as written by golangci-lint,
// ESLint's checkstyle formatter and many others (design §8.2).
package checkstyle

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
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

// severities is the builtin mapping for Checkstyle severity labels.
var severities = map[string]model.Severity{
	"error":   model.SeverityHigh,
	"warning": model.SeverityMedium,
	"info":    model.SeverityLow,
	"ignore":  model.SeverityInfo,
}

// Parse reads every <error> of every <file>.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, _ *gitctx.Context) (ingest.Result, error) {
	var rep report
	if err := xml.NewDecoder(r).Decode(&rep); err != nil {
		return ingest.Result{}, fmt.Errorf("decode checkstyle: %w", err)
	}
	var res ingest.Result
	unnamed := 0
	for _, fl := range rep.Files {
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
			f.Location.StartLine = atoi(e.Line)
			f.Location.EndLine = f.Location.StartLine
			f.Location.StartCol = atoi(e.Column)
			f.Source = model.Source{Kind: model.SourceTool}
			f.RawSeverity = e.Severity
			if s, ok := severities[e.Severity]; ok {
				f.DefaultSeverity = ingest.SeverityPtr(s)
			}
			res.Findings = append(res.Findings, f)
		}
	}
	if unnamed > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("rule ID %q given to errors without a source attribute: %d", unnamedRule, unnamed))
	}
	return res, nil
}

// atoi reads a non-negative integer attribute; anything else is 0, which
// means file-level for lines.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
