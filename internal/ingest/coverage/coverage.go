// Package coverage turns per-line coverage data into coverage metrics at
// repo and diff metric scope. The cobertura and lcov ingestors share it.
package coverage

import (
	"context"
	"fmt"
	"maps"
	"math"
	"path"
	"slices"
	"strings"

	"github.com/assimoes/qgate/internal/gitctx"
	"github.com/assimoes/qgate/internal/ingest"
	"github.com/assimoes/qgate/internal/model"
)

// Metric names.
const (
	LineMetric   = "coverage.line"
	BranchMetric = "coverage.branch"
)

// Branches are a line's branch outcomes.
type Branches struct {
	Covered, Total int
}

// File is one source file's coverage.
type File struct {
	// Paths are what the report's path for the file could mean, most
	// specific first: Cobertura's filename joined to each <source>, then
	// the filename alone.
	Paths    []string
	Lines    map[int]int      // line → hit count, for instrumented lines
	Branches map[int]Branches // line → branch outcomes, for lines with branches
}

// NewFile returns an empty File for the given candidate paths.
func NewFile(paths ...string) *File {
	return &File{Paths: paths, Lines: map[int]int{}, Branches: map[int]Branches{}}
}

// Files collects coverage by file, merging repeats: hits are summed,
// branches keep the larger count, as when one file appears in several
// Cobertura classes or LCOV records.
type Files map[string]*File

// Add merges f into the collection.
func (fs Files) Add(f *File) {
	key := strings.Join(f.Paths, "\x00")
	cur, ok := fs[key]
	if !ok {
		fs[key] = f
		return
	}
	for l, h := range f.Lines {
		cur.Lines[l] += h
	}
	for l, b := range f.Branches {
		c := cur.Branches[l]
		cur.Branches[l] = Branches{Covered: max(c.Covered, b.Covered), Total: max(c.Total, b.Total)}
	}
}

// Result computes the collection's metrics into an ingest result.
func (fs Files) Result(ctx context.Context, gc *gitctx.Context, warnings []string) (ingest.Result, error) {
	files := make([]*File, 0, len(fs))
	for _, k := range slices.Sorted(maps.Keys(fs)) {
		files = append(files, fs[k])
	}
	metrics, more, err := measure(ctx, files, gc)
	if err != nil {
		return ingest.Result{}, err
	}
	return ingest.Result{Metrics: metrics, Warnings: append(warnings, more...)}, nil
}

// tally counts covered and total units.
type tally struct{ covered, total int }

func (t *tally) add(covered, total int) { t.covered += covered; t.total += total }

func (t tally) percent() float64 {
	if t.total == 0 {
		return 100 // nothing to cover
	}
	return math.Round(float64(t.covered)*10000/float64(t.total)) / 100
}

// measure computes line and branch coverage at repo scope, and at diff
// scope over the lines the change added or modified. Branch metrics appear
// only when the report has branch data. With nothing instrumented on
// changed lines, diff coverage is 100: the change left nothing uncovered.
// gc may be nil (no diff), which yields repo metrics only.
func measure(ctx context.Context, files []*File, gc *gitctx.Context) ([]model.Metric, []string, error) {
	var repoLines, repoBranches, diffLines, diffBranches tally
	var warnings []string
	for _, f := range files {
		changed, warn, err := changedRanges(ctx, f, gc)
		if err != nil {
			return nil, nil, err
		}
		if warn != "" {
			warnings = append(warnings, warn)
		}
		for l, hits := range f.Lines {
			c := 0
			if hits > 0 {
				c = 1
			}
			repoLines.add(c, 1)
			if inRanges(l, changed) {
				diffLines.add(c, 1)
			}
		}
		for l, b := range f.Branches {
			repoBranches.add(b.Covered, b.Total)
			if inRanges(l, changed) {
				diffBranches.add(b.Covered, b.Total)
			}
		}
	}

	metric := func(name string, scope model.MetricScope, t tally) model.Metric {
		return model.Metric{Name: name, Value: t.percent(), Unit: model.UnitPercent, Scope: scope}
	}
	var out []model.Metric
	if repoLines.total > 0 {
		out = append(out, metric(LineMetric, model.MetricScopeRepo, repoLines))
		if gc != nil {
			out = append(out, metric(LineMetric, model.MetricScopeDiff, diffLines))
		}
	}
	if repoBranches.total > 0 {
		out = append(out, metric(BranchMetric, model.MetricScopeRepo, repoBranches))
		if gc != nil {
			out = append(out, metric(BranchMetric, model.MetricScopeDiff, diffBranches))
		}
	}
	return out, warnings, nil
}

// changedRanges finds which changed file, if any, a coverage file is.
// A candidate path that names a file in the repo is that file, changed or
// not. Otherwise the report wrote some other form of the path (an import
// path, another machine's absolute path), and the file is the changed file
// whose path is the longest suffix of a candidate; a tie is ambiguous and
// matches nothing, with a warning.
func changedRanges(ctx context.Context, f *File, gc *gitctx.Context) ([]gitctx.LineRange, string, error) {
	if gc == nil {
		return nil, "", nil
	}
	cleaned := make([]string, len(f.Paths))
	for i, p := range f.Paths {
		cleaned[i] = repoRelative(p, gc.Workspaces)
		if path.IsAbs(cleaned[i]) || strings.HasPrefix(cleaned[i], "../") {
			continue
		}
		ok, err := gc.HasFile(ctx, cleaned[i])
		if err != nil {
			return nil, "", fmt.Errorf("match coverage path %s: %w", p, err)
		}
		if ok {
			cf, _ := gc.File(cleaned[i])
			return liveRanges(cf), "", nil
		}
	}

	var best []gitctx.ChangedFile
	bestLen := 0
	for _, c := range cleaned {
		for _, cf := range gc.Files {
			if !strings.HasSuffix(c, "/"+cf.Path) || len(cf.Path) < bestLen {
				continue
			}
			if len(cf.Path) > bestLen {
				best, bestLen = nil, len(cf.Path)
			}
			if !slices.ContainsFunc(best, func(b gitctx.ChangedFile) bool { return b.Path == cf.Path }) {
				best = append(best, cf)
			}
		}
	}
	switch len(best) {
	case 0:
		return nil, "", nil
	case 1:
		return liveRanges(best[0]), "", nil
	default:
		names := make([]string, len(best))
		for i, b := range best {
			names[i] = b.Path
		}
		return nil, fmt.Sprintf("coverage for %s could be any of %s; it counts for none of them at diff scope",
			f.Paths[len(f.Paths)-1], strings.Join(names, ", ")), nil
	}
}

// liveRanges are a changed file's added ranges, none for deleted or binary
// files.
func liveRanges(cf gitctx.ChangedFile) []gitctx.LineRange {
	if cf.Status == gitctx.StatusDeleted || cf.Binary {
		return nil
	}
	return cf.AddedRanges
}

// repoRelative strips a workspace prefix and cleans the path.
func repoRelative(p string, workspaces []string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	for _, ws := range workspaces {
		if rest, ok := strings.CutPrefix(p, strings.TrimSuffix(ws, "/")+"/"); ok {
			p = rest
			break
		}
	}
	return path.Clean(p)
}

func inRanges(line int, ranges []gitctx.LineRange) bool {
	return slices.ContainsFunc(ranges, func(r gitctx.LineRange) bool { return line >= r.Start && line <= r.End })
}
