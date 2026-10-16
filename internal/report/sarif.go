package report

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/assimoes/qgate/internal/model"
)

// SARIF renders new findings as SARIF 2.1.0, one run per input or agent,
// for upload to code scanning.
type SARIF struct{}

// Name is the format name outputs use.
func (SARIF) Name() string { return "sarif" }

// securitySeverity is what SARIF consumers such as GitHub code scanning
// rank security results by; each value sits inside its severity's band.
var securitySeverity = map[model.Severity]string{
	model.SeverityCritical: "9.5",
	model.SeverityHigh:     "8.0",
	model.SeverityMedium:   "5.5",
	model.SeverityLow:      "2.0",
	model.SeverityInfo:     "0.0",
}

func sarifLevel(s model.Severity) string {
	switch {
	case s >= model.SeverityHigh:
		return "error"
	case s == model.SeverityMedium:
		return "warning"
	case s == model.SeverityLow:
		return "note"
	default:
		return "none"
	}
}

type (
	sarifLog struct {
		Schema  string     `json:"$schema"`
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}
	sarifRun struct {
		Tool    sarifTool     `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	sarifDriver struct {
		Name    string      `json:"name"`
		Version string      `json:"version,omitempty"`
		Rules   []sarifRule `json:"rules"`
	}
	sarifRule struct {
		ID               string         `json:"id"`
		ShortDescription sarifText      `json:"shortDescription"`
		Properties       map[string]any `json:"properties"`
	}
	sarifText struct {
		Text string `json:"text"`
	}
	sarifResult struct {
		RuleID              string            `json:"ruleId"`
		RuleIndex           int               `json:"ruleIndex"`
		Level               string            `json:"level"`
		Message             sarifText         `json:"message"`
		Locations           []sarifLocation   `json:"locations"`
		PartialFingerprints map[string]string `json:"partialFingerprints"`
	}
	sarifLocation struct {
		PhysicalLocation sarifPhysical `json:"physicalLocation"`
	}
	sarifPhysical struct {
		ArtifactLocation sarifArtifact `json:"artifactLocation"`
		Region           *sarifRegion  `json:"region,omitempty"`
	}
	sarifArtifact struct {
		URI string `json:"uri"`
	}
	sarifRegion struct {
		StartLine   int `json:"startLine"`
		EndLine     int `json:"endLine,omitempty"`
		StartColumn int `json:"startColumn,omitempty"`
	}
)

// Render encodes the report's new findings.
func (SARIF) Render(r *model.Report) ([]byte, error) {
	bySource := map[string][]model.Finding{}
	versions := map[string]string{}
	for _, f := range r.Findings {
		if f.Status != model.StatusNew {
			continue
		}
		bySource[f.Source.Name] = append(bySource[f.Source.Name], f)
		versions[f.Source.Name] = f.Source.Version
	}
	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs:    []sarifRun{},
	}
	for _, name := range slices.Sorted(maps.Keys(bySource)) {
		log.Runs = append(log.Runs, sarifRunFor(name, versions[name], bySource[name]))
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func sarifRunFor(name, version string, fs []model.Finding) sarifRun {
	run := sarifRun{Tool: sarifTool{Driver: sarifDriver{Name: name, Version: version, Rules: []sarifRule{}}}, Results: []sarifResult{}}
	ruleIndex := map[string]int{}
	for _, f := range fs {
		if _, ok := ruleIndex[f.RuleID]; !ok {
			ruleIndex[f.RuleID] = len(run.Tool.Driver.Rules)
			tags := append([]string{string(f.Category)}, f.CWE...)
			run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, sarifRule{
				ID:               f.RuleID,
				ShortDescription: sarifText{Text: firstNonEmpty(f.Title, f.RuleID)},
				Properties: map[string]any{
					"tags":              tags,
					"security-severity": securitySeverity[f.Severity],
				},
			})
		}
		phys := sarifPhysical{ArtifactLocation: sarifArtifact{URI: f.Location.Path}}
		if f.Location.StartLine > 0 {
			phys.Region = &sarifRegion{StartLine: f.Location.StartLine, EndLine: f.Location.EndLine, StartColumn: f.Location.StartCol}
		}
		run.Results = append(run.Results, sarifResult{
			RuleID:              f.RuleID,
			RuleIndex:           ruleIndex[f.RuleID],
			Level:               sarifLevel(f.Severity),
			Message:             sarifText{Text: firstNonEmpty(f.Message, f.Title, f.RuleID)},
			Locations:           []sarifLocation{{PhysicalLocation: phys}},
			PartialFingerprints: map[string]string{"qgate/v1": f.Fingerprint},
		})
	}
	slices.SortStableFunc(run.Results, func(a, b sarifResult) int { return strings.Compare(a.RuleID, b.RuleID) })
	return run
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
