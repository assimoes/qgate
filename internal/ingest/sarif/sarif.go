// Package sarif ingests SARIF 2.1.0 logs, the primary interchange format
// for analyzers (design §8.2).
package sarif

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
)

// Ingestor parses SARIF logs.
type Ingestor struct{}

// New returns the SARIF ingestor.
func New() Ingestor { return Ingestor{} }

// Format is the format name inputs use.
func (Ingestor) Format() string { return "sarif" }

// The subset of SARIF 2.1.0 qgate reads.
type (
	log struct {
		Version string `json:"version"`
		Runs    []run  `json:"runs"`
	}
	run struct {
		Tool    tool     `json:"tool"`
		Results []result `json:"results"`
		// OriginalURIBaseIDs resolve artifactLocation.uriBaseId.
		OriginalURIBaseIDs map[string]artifactLocation `json:"originalUriBaseIds"`
	}
	tool struct {
		Driver driver `json:"driver"`
		// Extensions hold rules from plugins or query packs; CodeQL
		// puts all its rules here and leaves the driver's empty.
		Extensions []driver `json:"extensions"`
	}
	driver struct {
		Name            string `json:"name"`
		Version         string `json:"version"`
		SemanticVersion string `json:"semanticVersion"`
		Rules           []rule `json:"rules"`
	}
	rule struct {
		ID                   string         `json:"id"`
		Name                 string         `json:"name"`
		ShortDescription     *text          `json:"shortDescription"`
		DefaultConfiguration *ruleConfig    `json:"defaultConfiguration"`
		Properties           properties     `json:"properties"`
		Relationships        []relationship `json:"relationships"`
	}
	ruleConfig struct {
		Level string `json:"level"`
	}
	relationship struct {
		Target struct {
			ID            string `json:"id"`
			ToolComponent struct {
				Name string `json:"name"`
			} `json:"toolComponent"`
		} `json:"target"`
	}
	result struct {
		RuleID              string            `json:"ruleId"`
		Kind                string            `json:"kind"`
		RuleIndex           *int              `json:"ruleIndex"`
		Rule                *ruleRef          `json:"rule"`
		Level               string            `json:"level"`
		Message             text              `json:"message"`
		Locations           []location        `json:"locations"`
		PartialFingerprints map[string]string `json:"partialFingerprints"`
		Fingerprints        map[string]string `json:"fingerprints"`
		Properties          properties        `json:"properties"`
	}
	ruleRef struct {
		ID            string `json:"id"`
		Index         *int   `json:"index"`
		ToolComponent *struct {
			Index *int `json:"index"`
		} `json:"toolComponent"`
	}
	text struct {
		Text string `json:"text"`
	}
	location struct {
		PhysicalLocation *physicalLocation `json:"physicalLocation"`
	}
	physicalLocation struct {
		ArtifactLocation artifactLocation `json:"artifactLocation"`
		Region           *region          `json:"region"`
	}
	artifactLocation struct {
		URI       string `json:"uri"`
		URIBaseID string `json:"uriBaseId"`
	}
	region struct {
		StartLine   int `json:"startLine"`
		EndLine     int `json:"endLine"`
		StartColumn int `json:"startColumn"`
		EndColumn   int `json:"endColumn"`
	}
	// properties is a SARIF property bag; values are kept raw because
	// analyzers disagree on types (security-severity is a string in some
	// and a number in others).
	properties map[string]json.RawMessage
)

// levelSeverity is the builtin mapping for SARIF result levels.
var levelSeverity = map[string]model.Severity{
	"error":   model.SeverityHigh,
	"warning": model.SeverityMedium,
	"note":    model.SeverityLow,
	"none":    model.SeverityInfo,
}

// Parse reads every run of a SARIF log.
func (Ingestor) Parse(_ context.Context, r io.Reader, _ config.Input, _ *gitctx.Context) (ingest.Result, error) {
	var l log
	if err := json.NewDecoder(r).Decode(&l); err != nil {
		return ingest.Result{}, fmt.Errorf("decode sarif: %w", err)
	}
	var res ingest.Result
	if l.Version != "2.1.0" {
		res.Warnings = append(res.Warnings, fmt.Sprintf("SARIF version %q, expected 2.1.0; reading it as 2.1.0", l.Version))
	}
	for ri, rn := range l.Runs {
		version := rn.Tool.Driver.Version
		if version == "" {
			version = rn.Tool.Driver.SemanticVersion
		}
		rules := indexRules(rn.Tool)
		notDefects := 0
		for i, rs := range rn.Results {
			// Only kind "fail" (the default) is a defect; pass,
			// notApplicable, informational, review and open are not.
			if rs.Kind != "" && rs.Kind != "fail" {
				notDefects++
				continue
			}
			f, warn, ok := convert(rs, rules, rn.OriginalURIBaseIDs)
			if warn != "" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("run %d (%s) result %d: %s", ri, rn.Tool.Driver.Name, i, warn))
			}
			if !ok {
				continue
			}
			f.Source.Version = version
			res.Findings = append(res.Findings, f)
		}
		if notDefects > 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf(
				"run %d (%s): skipped %d results whose kind is not \"fail\"", ri, rn.Tool.Driver.Name, notDefects))
		}
	}
	return res, nil
}

// ruleIndex finds the rule a result refers to, in the driver or in one of
// the tool's extensions.
type ruleIndex struct {
	driver     []rule
	extensions [][]rule
	byID       map[string]*rule // every rule, driver first
}

func indexRules(t tool) ruleIndex {
	idx := ruleIndex{driver: t.Driver.Rules, byID: map[string]*rule{}}
	for _, ext := range t.Extensions {
		idx.extensions = append(idx.extensions, ext.Rules)
	}
	for _, rules := range append([][]rule{idx.driver}, idx.extensions...) {
		for i := range rules {
			if _, seen := idx.byID[rules[i].ID]; !seen {
				idx.byID[rules[i].ID] = &rules[i]
			}
		}
	}
	return idx
}

func (idx ruleIndex) find(rs result) (string, *rule) {
	id, pos, rules := rs.RuleID, rs.RuleIndex, idx.driver
	if ref := rs.Rule; ref != nil {
		if id == "" {
			id = ref.ID
		}
		if ref.Index != nil {
			pos = ref.Index
		}
		if tc := ref.ToolComponent; tc != nil && tc.Index != nil {
			rules = nil
			if i := *tc.Index; i >= 0 && i < len(idx.extensions) {
				rules = idx.extensions[i]
			}
		}
	}
	if pos != nil && *pos >= 0 && *pos < len(rules) && (id == "" || rules[*pos].ID == id) {
		return rules[*pos].ID, &rules[*pos]
	}
	if r, ok := idx.byID[id]; ok && id != "" {
		return id, r
	}
	return id, nil
}

// convert maps one result. It returns ok=false for results qgate cannot
// place, with a warning saying why.
func convert(rs result, rules ruleIndex, bases map[string]artifactLocation) (ingest.Finding, string, bool) {
	id, rl := rules.find(rs)
	if id == "" {
		return ingest.Finding{}, "result has no rule ID; skipped", false
	}
	if len(rs.Locations) == 0 || rs.Locations[0].PhysicalLocation == nil ||
		rs.Locations[0].PhysicalLocation.ArtifactLocation.URI == "" {
		return ingest.Finding{}, fmt.Sprintf("result for rule %s has no file location; skipped", id), false
	}
	phys := rs.Locations[0].PhysicalLocation

	f := ingest.Finding{}
	f.RuleID = id
	f.Message = rs.Message.Text
	f.Title = title(id, rl)
	p, err := resolvePath(phys.ArtifactLocation, bases)
	if err != nil {
		return ingest.Finding{}, fmt.Sprintf("result for rule %s: %v; skipped", id, err), false
	}
	f.Location.Path = p
	if rg := phys.Region; rg != nil && rg.StartLine > 0 {
		f.Location.StartLine = rg.StartLine
		f.Location.EndLine = max(rg.EndLine, rg.StartLine)
		f.Location.StartCol = rg.StartColumn
		f.Location.EndCol = rg.EndColumn
	}
	f.Source = model.Source{Kind: model.SourceTool}

	// Severity: the level is the label severity_map keys match; the builtin
	// default prefers security-severity, which is finer grained.
	level := rs.Level
	if level == "" && rl != nil && rl.DefaultConfiguration != nil {
		level = rl.DefaultConfiguration.Level
	}
	if level == "" {
		level = "warning" // the SARIF default
	}
	f.RawSeverity = level
	if score, ok := securitySeverity(rs.Properties, rl); ok {
		f.DefaultSeverity = ingest.SeverityPtr(scoreSeverity(score))
	} else if s, ok := levelSeverity[level]; ok {
		f.DefaultSeverity = ingest.SeverityPtr(s)
	}

	f.CWE = cwes(rl)
	f.ToolFingerprint = toolFingerprint(rs)
	return f, "", true
}

// maxBaseDepth bounds uriBaseId chains, which may refer to each other.
const maxBaseDepth = 8

// resolvePath turns an artifact location into a file path: the URI is
// percent-decoded, and a uriBaseId is resolved through the run's
// originalUriBaseIds. A base of the filesystem root ("file:///") adds no
// information, so a URI relative to it stays relative. The result is
// absolute or repo-relative; normalization makes it repo-relative.
func resolvePath(loc artifactLocation, bases map[string]artifactLocation) (string, error) {
	u, err := url.Parse(loc.URI)
	if err != nil {
		return "", fmt.Errorf("invalid artifact URI %q", loc.URI)
	}
	for depth := 0; !u.IsAbs() && loc.URIBaseID != "" && depth < maxBaseDepth; depth++ {
		base, ok := bases[loc.URIBaseID]
		if !ok || base.URI == "" {
			break // an unknown base is resolved by the analyzer's working directory
		}
		bu, err := url.Parse(base.URI)
		if err != nil {
			return "", fmt.Errorf("invalid base URI %q for %s", base.URI, loc.URIBaseID)
		}
		if bu.Scheme == "file" && (bu.Path == "/" || bu.Path == "") {
			break
		}
		if !strings.HasSuffix(bu.Path, "/") {
			bu.Path += "/"
		}
		u = bu.ResolveReference(u)
		loc = base
	}
	if u.Scheme != "" && u.Scheme != "file" {
		return "", fmt.Errorf("artifact %q is not a file", loc.URI)
	}
	return u.Path, nil
}

func title(id string, rl *rule) string {
	switch {
	case rl != nil && rl.ShortDescription != nil && rl.ShortDescription.Text != "":
		return rl.ShortDescription.Text
	case rl != nil && rl.Name != "":
		return rl.Name
	default:
		return id
	}
}

// securitySeverity reads the CVSS-like security-severity property from the
// result, falling back to its rule.
func securitySeverity(resProps properties, rl *rule) (float64, bool) {
	bags := []properties{resProps}
	if rl != nil {
		bags = append(bags, rl.Properties)
	}
	for _, bag := range bags {
		raw, ok := bag["security-severity"]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return v, true
			}
			continue
		}
		var v float64
		if json.Unmarshal(raw, &v) == nil {
			return v, true
		}
	}
	return 0, false
}

func scoreSeverity(score float64) model.Severity {
	switch {
	case score >= 9.0:
		return model.SeverityCritical
	case score >= 7.0:
		return model.SeverityHigh
	case score >= 4.0:
		return model.SeverityMedium
	default:
		return model.SeverityLow
	}
}

var cweTag = regexp.MustCompile(`(?i)cwe[-/:]0*(\d+)`)

// cwes collects CWE IDs from a rule's tags and its relationships to the CWE
// taxonomy, as "CWE-<n>", sorted and deduplicated.
func cwes(rl *rule) []string {
	if rl == nil {
		return nil
	}
	set := map[string]bool{}
	if raw, ok := rl.Properties["tags"]; ok {
		var tags []string
		if json.Unmarshal(raw, &tags) == nil {
			for _, t := range tags {
				if m := cweTag.FindStringSubmatch(t); m != nil {
					set["CWE-"+m[1]] = true
				}
			}
		}
	}
	for _, rel := range rl.Relationships {
		if strings.EqualFold(rel.Target.ToolComponent.Name, "CWE") {
			if m := cweTag.FindStringSubmatch("cwe-" + strings.TrimPrefix(strings.ToUpper(rel.Target.ID), "CWE-")); m != nil {
				set["CWE-"+m[1]] = true
			}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}

// toolFingerprint combines the result's fingerprints into one stable
// string: full fingerprints if present, else partial ones, in key order.
func toolFingerprint(rs result) string {
	fps := rs.Fingerprints
	if len(fps) == 0 {
		fps = rs.PartialFingerprints
	}
	if len(fps) == 0 {
		return ""
	}
	keys := slices.Sorted(maps.Keys(fps))
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + fps[k]
	}
	return strings.Join(parts, ";")
}
