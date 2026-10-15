// Package normalize turns ingested findings into normalized, classified
// findings (design §9). Each step is its own function and the steps run in
// the design's order.
package normalize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
)

// defaultCategory applies when neither a category mapping nor the input's
// category_default does.
const defaultCategory model.Category = "quality"

// LineSource reads a file's lines at the head commit; nil lines mean the
// file is not there.
type LineSource interface {
	FileLines(ctx context.Context, path string) ([]string, error)
}

// Options are what normalizing one input's findings needs.
type Options struct {
	Input       config.Input
	Project     config.Project
	GatingScope model.GatingScope
	Changes     *gitctx.Context
	Lines       LineSource
	// Workspaces are absolute directories analyzers may have prefixed
	// paths with: the repo root and the CI workspace.
	Workspaces []string
}

// Tool normalizes one input's findings: paths, rule namespacing, severity,
// category, fingerprint and gating scope (design §9 steps 1–6). It returns
// the findings in canonical order, plus warnings for the report.
func Tool(ctx context.Context, in []ingest.Finding, opts Options) ([]model.Finding, []string, error) {
	var warnings []string
	kept := normalizePaths(in, opts)
	namespaceRules(kept, opts.Input.Name)
	warnings = append(warnings, mapSeverities(kept, opts.Input.SeverityMap)...)
	assignCategories(kept, opts.Input.CategoryDefault)

	// Canonical order first, so occurrence indexes in fingerprints are
	// deterministic.
	sort.SliceStable(kept, func(i, j int) bool { return canonicalLess(kept[i].Finding, kept[j].Finding) })
	if err := fingerprint(ctx, kept, opts.Lines); err != nil {
		return nil, nil, err
	}

	out := make([]model.Finding, len(kept))
	for i := range kept {
		out[i] = kept[i].Finding
	}
	classifyScope(out, opts.GatingScope, opts.Changes)
	sort.SliceStable(out, func(i, j int) bool { return canonicalLess(out[i], out[j]) })
	return out, warnings, nil
}

// normalizePaths makes paths repo-relative with forward slashes and drops
// findings outside the project (step 1).
func normalizePaths(in []ingest.Finding, opts Options) []ingest.Finding {
	out := make([]ingest.Finding, 0, len(in))
	for _, f := range in {
		p := repoRelative(f.Location.Path, opts.Workspaces)
		if p == "" || !gitctx.Selected(p, opts.Project.Include, opts.Project.Exclude) {
			continue
		}
		f.Location.Path = p
		out = append(out, f)
	}
	return out
}

func repoRelative(p string, workspaces []string) string {
	if u, err := url.Parse(p); err == nil && u.Scheme == "file" {
		p = u.Path
	}
	p = strings.ReplaceAll(p, `\`, "/")
	for _, ws := range workspaces {
		ws = strings.TrimSuffix(strings.ReplaceAll(ws, `\`, "/"), "/")
		if ws != "" && strings.HasPrefix(p, ws+"/") {
			p = strings.TrimPrefix(p, ws+"/")
			break
		}
	}
	p = path.Clean(strings.TrimPrefix(p, "./"))
	if p == "." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		return ""
	}
	return p
}

// namespaceRules prefixes rule IDs with the input name (step 2).
func namespaceRules(fs []ingest.Finding, input string) {
	for i := range fs {
		fs[i].RuleID = input + ":" + fs[i].RuleID
	}
}

// mapSeverities applies the input's severity_map, then the format's
// builtin mapping; anything left is medium, with a warning (step 3).
func mapSeverities(fs []ingest.Finding, severityMap map[string]model.Severity) []string {
	unmapped := map[string]bool{}
	for i := range fs {
		f := &fs[i]
		if s, ok := severityMap[f.RawSeverity]; ok {
			f.Severity = s
			continue
		}
		if f.DefaultSeverity != nil {
			f.Severity = *f.DefaultSeverity
			continue
		}
		f.Severity = model.SeverityMedium
		unmapped[f.RawSeverity] = true
	}
	labels := make([]string, 0, len(unmapped))
	for l := range unmapped {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	warnings := make([]string, len(labels))
	for i, l := range labels {
		warnings[i] = fmt.Sprintf("unmapped severity %q treated as medium; map it with severity_map", l)
	}
	return warnings
}

// assignCategories sets each finding's category (step 4). Category
// mappings arrive with ticket 07.
func assignCategories(fs []ingest.Finding, categoryDefault model.Category) {
	cat := categoryDefault
	if cat == "" {
		cat = defaultCategory
	}
	for i := range fs {
		fs[i].Category = cat
	}
}

// fingerprint gives each finding a stable identity that survives lines
// moving (step 5). Line numbers never enter it.
func fingerprint(ctx context.Context, fs []ingest.Finding, lines LineSource) error {
	files := map[string][]string{}
	occurrences := map[string]int{}
	for i := range fs {
		f := &fs[i]
		if f.ToolFingerprint != "" {
			f.Fingerprint = hash(f.RuleID, f.ToolFingerprint)
			continue
		}
		content, err := lineContent(ctx, files, lines, f.Location)
		if err != nil {
			return err
		}
		key := f.RuleID + "\x00" + f.Location.Path + "\x00" + content
		n := occurrences[key]
		occurrences[key] = n + 1
		f.Fingerprint = hash(f.RuleID, f.Location.Path, content, strconv.Itoa(n))
	}
	return nil
}

func lineContent(ctx context.Context, cache map[string][]string, src LineSource, loc model.Location) (string, error) {
	if loc.StartLine == 0 {
		return "", nil
	}
	ls, ok := cache[loc.Path]
	if !ok {
		var err error
		if ls, err = src.FileLines(ctx, loc.Path); err != nil {
			return "", fmt.Errorf("read %s for fingerprinting: %w", loc.Path, err)
		}
		cache[loc.Path] = ls
	}
	if loc.StartLine > len(ls) {
		return "", nil
	}
	return strings.Join(strings.Fields(ls[loc.StartLine-1]), " "), nil
}

func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// classifyScope marks each finding new or out_of_scope (step 6).
func classifyScope(fs []model.Finding, scope model.GatingScope, changes *gitctx.Context) {
	for i := range fs {
		if inScope(fs[i].Location, scope, changes) {
			fs[i].Status = model.StatusNew
		} else {
			fs[i].Status = model.StatusOutOfScope
		}
	}
}

func inScope(loc model.Location, scope model.GatingScope, changes *gitctx.Context) bool {
	if scope == model.GatingScopeRepo {
		return true
	}
	file, changed := changes.File(loc.Path)
	if !changed || file.Status == gitctx.StatusDeleted {
		return false
	}
	if scope == model.GatingScopeFiles || loc.StartLine == 0 {
		return true
	}
	end := max(loc.EndLine, loc.StartLine)
	for _, r := range file.AddedRanges {
		if loc.StartLine <= r.End && end >= r.Start {
			return true
		}
	}
	return false
}

// canonicalLess orders findings by path, line, rule and then fingerprint,
// so output never depends on input order.
func canonicalLess(a, b model.Finding) bool {
	switch {
	case a.Location.Path != b.Location.Path:
		return a.Location.Path < b.Location.Path
	case a.Location.StartLine != b.Location.StartLine:
		return a.Location.StartLine < b.Location.StartLine
	case a.Location.EndLine != b.Location.EndLine:
		return a.Location.EndLine < b.Location.EndLine
	case a.RuleID != b.RuleID:
		return a.RuleID < b.RuleID
	case a.Location.StartCol != b.Location.StartCol:
		return a.Location.StartCol < b.Location.StartCol
	case a.Message != b.Message:
		return a.Message < b.Message
	default:
		return a.Fingerprint < b.Fingerprint
	}
}
