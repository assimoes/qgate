// Package normalize turns ingested findings into normalized, classified
// findings (design §9). Each step is its own function and the steps run in
// the design's order.
package normalize

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
)

// ChangeSet says how the change under review touched a file.
type ChangeSet interface {
	File(path string) (gitctx.ChangedFile, bool)
}

// LineSource reads a file's lines at the head commit; nil lines mean the
// file is not there.
type LineSource interface {
	FileLines(ctx context.Context, path string) ([]string, error)
}

// Options are what normalizing one input's findings needs.
type Options struct {
	Input            config.Input
	CategoryMappings []config.CategoryMapping
	Project          config.Project
	GatingScope      model.GatingScope
	Changes          ChangeSet
	Lines            LineSource
	// Workspaces are absolute directories analyzers may have prefixed
	// paths with: the repo root and the CI workspace.
	Workspaces []string
}

// ToolFindings normalizes one input's findings: paths, rule namespacing, severity,
// category, fingerprint and gating scope (design §9 steps 1–6). It returns
// the findings in canonical order, plus warnings for the report.
func ToolFindings(ctx context.Context, in []ingest.Finding, opts Options) ([]model.Finding, []string, error) {
	var warnings []string
	kept := normalizePaths(in, opts)
	namespaceRules(kept, opts.Input.Name)
	warnings = append(warnings, mapSeverities(kept, opts.Input.SeverityMap)...)
	assignCategories(kept, opts.CategoryMappings, opts.Input.EffectiveCategory())

	// Canonical order first, so occurrence indexes in fingerprints are
	// deterministic.
	slices.SortStableFunc(kept, func(a, b ingest.Finding) int { return compareCanonical(a.Finding, b.Finding) })
	if err := fingerprint(ctx, kept, opts.Lines); err != nil {
		return nil, nil, err
	}

	out := make([]model.Finding, len(kept))
	for i := range kept {
		out[i] = kept[i].Finding
	}
	classifyScope(out, opts.GatingScope, opts.Changes)
	slices.SortStableFunc(out, compareCanonical)
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
	labels := slices.Sorted(maps.Keys(unmapped))
	warnings := make([]string, len(labels))
	for i, l := range labels {
		warnings[i] = fmt.Sprintf("unmapped severity %q treated as medium; map it with severity_map", l)
	}
	return warnings
}

// assignCategories sets each finding's category (step 4): the first
// category mapping that matches, else the input's effective category.
func assignCategories(fs []ingest.Finding, mappings []config.CategoryMapping, fallback model.Category) {
	for i := range fs {
		fs[i].Category = fallback
		for _, m := range mappings {
			if matches(m.Match, fs[i].Finding) {
				fs[i].Category = m.Category
				break
			}
		}
	}
}

// matches reports whether a finding satisfies every criterion a mapping
// sets. Rule IDs are matched after namespacing; cwe matches any of the
// finding's CWEs, and "*" any finding that has one.
func matches(m config.CategoryMatch, f model.Finding) bool {
	if m.RuleID != "" && !glob(m.RuleID, f.RuleID) {
		return false
	}
	if m.Path != "" && !glob(m.Path, f.Location.Path) {
		return false
	}
	if m.CWE != "" {
		return slices.ContainsFunc(f.CWE, func(c string) bool { return glob(m.CWE, c) })
	}
	return true
}

// glob matches with doublestar semantics; patterns were validated when the
// config was loaded.
func glob(pattern, s string) bool {
	ok, _ := doublestar.Match(pattern, s)
	return ok
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
func classifyScope(fs []model.Finding, scope model.GatingScope, changes ChangeSet) {
	for i := range fs {
		if inScope(fs[i].Location, scope, changes) {
			fs[i].Status = model.StatusNew
		} else {
			fs[i].Status = model.StatusOutOfScope
		}
	}
}

func inScope(loc model.Location, scope model.GatingScope, changes ChangeSet) bool {
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

// compareCanonical totally orders findings by path, line, rule, column,
// input, fingerprint and message, so output never depends on input order.
func compareCanonical(a, b model.Finding) int {
	return cmp.Or(
		cmp.Compare(a.Location.Path, b.Location.Path),
		cmp.Compare(a.Location.StartLine, b.Location.StartLine),
		cmp.Compare(a.Location.EndLine, b.Location.EndLine),
		cmp.Compare(a.RuleID, b.RuleID),
		cmp.Compare(a.Location.StartCol, b.Location.StartCol),
		cmp.Compare(a.Source.Name, b.Source.Name),
		cmp.Compare(a.Fingerprint, b.Fingerprint),
		cmp.Compare(a.Message, b.Message),
	)
}
