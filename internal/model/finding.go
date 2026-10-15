// Package model holds qgate's domain types. They are also the plugin and
// report wire format, versioned by the report's schema_version.
package model

import (
	"fmt"
)

// Severity orders how serious a finding is. It serializes as a lowercase string.
type Severity int

// Severities from least to most serious.
const (
	SeverityInfo Severity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

var severityNames = [...]string{"info", "low", "medium", "high", "critical"}

func (s Severity) valid() bool { return s >= SeverityInfo && s <= SeverityCritical }

// String returns the lowercase severity name.
func (s Severity) String() string {
	if !s.valid() {
		return fmt.Sprintf("Severity(%d)", int(s))
	}
	return severityNames[s]
}

// ParseSeverity parses a lowercase severity name.
func ParseSeverity(name string) (Severity, error) {
	for i, n := range severityNames {
		if n == name {
			return Severity(i), nil
		}
	}
	return 0, fmt.Errorf("unknown severity %q", name)
}

// MarshalText encodes the severity as its lowercase name. It also makes
// Severity usable as a JSON object key.
func (s Severity) MarshalText() ([]byte, error) {
	if !s.valid() {
		return nil, fmt.Errorf("marshal severity: invalid value %d", int(s))
	}
	return []byte(s.String()), nil
}

// UnmarshalText decodes a lowercase severity name.
func (s *Severity) UnmarshalText(text []byte) error {
	v, err := ParseSeverity(string(text))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// Category is a quality dimension scored separately. It is an open set; the
// builtins are security, quality, maintainability, testing and dependencies.
type Category string

// DefaultCategory is where a tool finding goes when neither a category
// mapping nor its input's category_default places it, and what an input
// with no category_default assesses.
const DefaultCategory Category = "quality"

// Location is where a finding is, on the new side of the change.
type Location struct {
	Path      string `json:"path"`       // repo-relative, forward slashes
	StartLine int    `json:"start_line"` // 1-based; 0 means file-level
	EndLine   int    `json:"end_line"`   // inclusive
	StartCol  int    `json:"start_col,omitempty"`
	EndCol    int    `json:"end_col,omitempty"`
}

// SourceKind says whether a finding came from an input or an agent.
type SourceKind string

// Source kinds.
const (
	SourceTool  SourceKind = "tool"
	SourceAgent SourceKind = "agent"
)

// Source identifies the input or agent that produced a finding or metric.
type Source struct {
	Kind    SourceKind `json:"kind"`
	Name    string     `json:"name"`              // input or agent name
	Version string     `json:"version,omitempty"` // analyzer version, or model id
}

// Status says whether a finding counts toward scores and the gate. Only
// StatusNew does.
type Status string

// Finding statuses.
const (
	StatusNew        Status = "new"
	StatusOutOfScope Status = "out_of_scope"
	StatusBaseline   Status = "baseline"
	StatusSuppressed Status = "suppressed"
	StatusSuperseded Status = "superseded"
	StatusInvalid    Status = "invalid"
)

// Suggestion is a proposed replacement for a line range, posted for a human
// to accept and never applied automatically.
type Suggestion struct {
	Description string `json:"description"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	Replacement string `json:"replacement"`
}

// Verdict is a triager's opinion of a finding.
type Verdict string

// Triage verdicts.
const (
	VerdictLikelyTrue          Verdict = "likely_true"
	VerdictLikelyFalsePositive Verdict = "likely_false_positive"
	VerdictUncertain           Verdict = "uncertain"
)

// Annotation is commentary an agent attaches to a finding.
type Annotation struct {
	Agent       string  `json:"agent"`
	Verdict     Verdict `json:"verdict,omitempty"` // triagers only
	Explanation string  `json:"explanation"`
}

// MergedSource records a tool finding folded into another during dedup.
type MergedSource struct {
	Input       string   `json:"input"`
	RuleID      string   `json:"rule_id"`
	Fingerprint string   `json:"fingerprint"`
	Severity    Severity `json:"severity"`
}

// Finding is a located issue with a rule ID, severity and category.
type Finding struct {
	Fingerprint   string         `json:"fingerprint"`
	RuleID        string         `json:"rule_id"`
	Category      Category       `json:"category"`
	Severity      Severity       `json:"severity"`
	Title         string         `json:"title"`
	Message       string         `json:"message"`
	Location      Location       `json:"location"`
	CWE           []string       `json:"cwe,omitempty"`
	Source        Source         `json:"source"`
	Confidence    *int           `json:"confidence,omitempty"` // 0–100, agent findings only
	Suggestion    *Suggestion    `json:"suggestion,omitempty"`
	Status        Status         `json:"status"`
	StatusNote    string         `json:"status_note,omitempty"`
	Annotations   []Annotation   `json:"annotations,omitempty"`
	MergedSources []MergedSource `json:"merged_sources,omitempty"`
}

// Metric is a numeric measurement with a metric scope: "diff", "repo" or
// "file:<path>".
type Metric struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Scope  string  `json:"scope"`
	Source Source  `json:"source"`
}
