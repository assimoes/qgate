package model

import "cmp"

// MetricScope is what a metric measures over: the diff, the whole repo, or
// one file ("file:<path>").
type MetricScope string

// Metric scopes.
const (
	MetricScopeDiff MetricScope = "diff"
	MetricScopeRepo MetricScope = "repo"
)

// Metric units.
const (
	UnitPercent = "percent"
	UnitCount   = "count"
)

// Metric is a numeric measurement with a metric scope.
type Metric struct {
	Name   string      `json:"name"`
	Value  float64     `json:"value"`
	Unit   string      `json:"unit"`
	Scope  MetricScope `json:"scope"`
	Source Source      `json:"source"`
}

// CompareMetrics orders metrics by name, scope and source, the canonical
// order reports use.
func CompareMetrics(a, b Metric) int {
	return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Scope, b.Scope), cmp.Compare(a.Source.Name, b.Source.Name))
}
