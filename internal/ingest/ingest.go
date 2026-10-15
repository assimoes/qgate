// Package ingest turns analyzer output into findings and metrics. Each
// report format is an Ingestor in its own package; the registry selects one
// by format name, so adding a format never edits existing code.
package ingest

import (
	"context"
	"io"
	"sort"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/model"
)

// Finding is a finding as an ingestor read it, before normalization.
// RuleID is the analyzer's own rule ID, not yet namespaced, and Severity is
// not yet set: normalization decides it from RawSeverity and DefaultSeverity.
type Finding struct {
	model.Finding
	// RawSeverity is the analyzer's own severity label, matched against the
	// input's severity_map.
	RawSeverity string `json:"raw_severity"`
	// DefaultSeverity is the format's builtin mapping for this finding, or
	// nil when the format has no mapping for its label.
	DefaultSeverity *model.Severity `json:"default_severity"`
	// ToolFingerprint is the analyzer's stable identity for the finding, if
	// it supplies one.
	ToolFingerprint string `json:"tool_fingerprint,omitempty"`
}

// Result is what one input produced.
type Result struct {
	Findings []Finding      `json:"findings"`
	Metrics  []model.Metric `json:"metrics"`
	Warnings []string       `json:"warnings"` // non-fatal problems, surfaced in the report
}

// Ingestor parses one report format.
type Ingestor interface {
	Format() string
	Parse(ctx context.Context, r io.Reader, in config.Input, gc *gitctx.Context) (Result, error)
}

// Registry finds the ingestor for a format.
type Registry struct {
	byFormat map[string]Ingestor
}

// NewRegistry registers the given ingestors by their format names.
func NewRegistry(ingestors ...Ingestor) *Registry {
	r := &Registry{byFormat: make(map[string]Ingestor, len(ingestors))}
	for _, in := range ingestors {
		r.byFormat[in.Format()] = in
	}
	return r
}

// Lookup returns the ingestor for format.
func (r *Registry) Lookup(format string) (Ingestor, bool) {
	in, ok := r.byFormat[format]
	return in, ok
}

// Formats lists the registered formats.
func (r *Registry) Formats() []string {
	out := make([]string, 0, len(r.byFormat))
	for f := range r.byFormat {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// SeverityPtr returns a pointer to s, for DefaultSeverity.
func SeverityPtr(s model.Severity) *model.Severity { return &s }
