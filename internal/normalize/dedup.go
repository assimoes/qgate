package normalize

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"qgate/internal/model"
)

// DedupTools merges tool findings from different inputs that report the
// same issue (design §9 step 9): identical fingerprints, or the same path,
// start line and non-empty CWE set. Each group keeps one survivor, the
// highest severity, ties going to the alphabetically first input; the
// others become its merged sources. The result is in canonical order and
// does not depend on the order of its input.
func DedupTools(fs []model.Finding) []model.Finding {
	fs = slices.Clone(fs)
	slices.SortStableFunc(fs, compareCanonical)

	parent := make([]int, len(fs))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	union := func(i, j int) {
		ri, rj := find(i), find(j)
		if ri != rj {
			parent[max(ri, rj)] = min(ri, rj)
		}
	}

	// Link findings from different inputs that share a key.
	byKey := map[string][]int{}
	for i, f := range fs {
		if f.Source.Kind != model.SourceTool {
			continue
		}
		byKey["fp\x00"+f.Fingerprint] = append(byKey["fp\x00"+f.Fingerprint], i)
		if len(f.CWE) > 0 {
			k := "loc\x00" + f.Location.Path + "\x00" + strconv.Itoa(f.Location.StartLine) + "\x00" + cweSet(f.CWE)
			byKey[k] = append(byKey[k], i)
		}
	}
	for _, idx := range byKey {
		for a, i := range idx {
			for _, j := range idx[a+1:] {
				if fs[i].Source.Name != fs[j].Source.Name {
					union(i, j)
				}
			}
		}
	}

	groups := map[int][]int{}
	for i := range fs {
		r := find(i)
		groups[r] = append(groups[r], i)
	}
	out := make([]model.Finding, 0, len(groups))
	for _, members := range groups {
		out = append(out, merge(fs, members))
	}
	slices.SortStableFunc(out, compareCanonical)
	return out
}

// merge returns the group's survivor with the others as merged sources.
func merge(fs []model.Finding, members []int) model.Finding {
	if len(members) == 1 {
		return fs[members[0]]
	}
	best := members[0]
	for _, i := range members[1:] {
		if survives(fs[i], fs[best]) {
			best = i
		}
	}
	survivor := fs[best]
	for _, i := range members {
		if i == best {
			continue
		}
		f := fs[i]
		survivor.MergedSources = append(survivor.MergedSources, model.MergedSource{
			Input: f.Source.Name, RuleID: f.RuleID, Fingerprint: f.Fingerprint, Severity: f.Severity,
		})
		// Already-merged sources of a member carry over.
		survivor.MergedSources = append(survivor.MergedSources, f.MergedSources...)
	}
	slices.SortFunc(survivor.MergedSources, func(a, b model.MergedSource) int {
		return cmp.Or(cmp.Compare(a.Input, b.Input), cmp.Compare(a.RuleID, b.RuleID), cmp.Compare(a.Fingerprint, b.Fingerprint))
	})
	return survivor
}

// survives reports whether a beats b: higher severity, then the
// alphabetically first input, then canonical order.
func survives(a, b model.Finding) bool {
	if a.Severity != b.Severity {
		return a.Severity > b.Severity
	}
	if a.Source.Name != b.Source.Name {
		return a.Source.Name < b.Source.Name
	}
	return compareCanonical(a, b) < 0
}

func cweSet(cwes []string) string {
	s := slices.Clone(cwes)
	slices.Sort(s)
	return strings.Join(slices.Compact(s), ",")
}
