// Package coverage turns per-line coverage data into coverage metrics at
// repo and diff metric scope. The cobertura and lcov ingestors share it.
package coverage

import (
	"math"
	"path"
	"slices"
	"strings"

	"qgate/internal/gitctx"
	"qgate/internal/model"
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
	Path     string           // as the report wrote it
	Lines    map[int]int      // line → hit count, for instrumented lines
	Branches map[int]Branches // line → branch outcomes, for lines with branches
}

// Merge adds other's data into f: hits are summed, branches keep the larger
// count, as when one file appears in several classes or records.
func (f *File) Merge(other File) {
	for l, h := range other.Lines {
		f.Lines[l] += h
	}
	for l, b := range other.Branches {
		cur := f.Branches[l]
		f.Branches[l] = Branches{Covered: max(cur.Covered, b.Covered), Total: max(cur.Total, b.Total)}
	}
}

// NewFile returns an empty File for p.
func NewFile(p string) File {
	return File{Path: p, Lines: map[int]int{}, Branches: map[int]Branches{}}
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

// Metrics computes line and branch coverage at repo scope, and at diff
// scope over the lines the change added or modified. Branch metrics appear
// only when the report has branch data. With nothing instrumented on
// changed lines, diff coverage is 100: the change left nothing uncovered.
// gc may be nil (no diff), which yields repo metrics only.
func Metrics(files []File, gc *gitctx.Context) []model.Metric {
	var repoLines, repoBranches, diffLines, diffBranches tally
	for _, f := range files {
		changed := changedRanges(f.Path, gc)
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

	metric := func(name, scope string, t tally) model.Metric {
		return model.Metric{Name: name, Value: t.percent(), Unit: "percent", Scope: scope}
	}
	var out []model.Metric
	if repoLines.total > 0 {
		out = append(out, metric(LineMetric, "repo", repoLines))
		if gc != nil {
			out = append(out, metric(LineMetric, "diff", diffLines))
		}
	}
	if repoBranches.total > 0 {
		out = append(out, metric(BranchMetric, "repo", repoBranches))
		if gc != nil {
			out = append(out, metric(BranchMetric, "diff", diffBranches))
		}
	}
	return out
}

// changedRanges finds the changed file a coverage path refers to. Coverage
// tools write paths relative to the repo, absolute, or as import paths
// ("example.com/org/app/a.go"), so a path matches a changed file when it
// equals it, or ends with "/" and the changed file's path, after workspace
// prefixes are removed.
func changedRanges(p string, gc *gitctx.Context) []gitctx.LineRange {
	if gc == nil {
		return nil
	}
	p = strings.ReplaceAll(p, `\`, "/")
	for _, ws := range gc.Workspaces {
		if rest, ok := strings.CutPrefix(p, strings.TrimSuffix(ws, "/")+"/"); ok {
			p = rest
			break
		}
	}
	p = path.Clean(p)
	for _, f := range gc.Files {
		if f.Status == gitctx.StatusDeleted || f.Binary {
			continue
		}
		if p == f.Path || strings.HasSuffix(p, "/"+f.Path) {
			return f.AddedRanges
		}
	}
	return nil
}

func inRanges(line int, ranges []gitctx.LineRange) bool {
	return slices.ContainsFunc(ranges, func(r gitctx.LineRange) bool { return line >= r.Start && line <= r.End })
}
