package normalize

import (
	"cmp"
	"slices"
	"strings"

	"qgate/internal/model"
)

// dedupKey is one way two findings can be the same issue: the same
// fingerprint, or the same path, start line and non-empty CWE set.
type dedupKey struct {
	fingerprint string
	path        string
	line        int
	cwes        string
}

func dedupKeys(f model.Finding) []dedupKey {
	keys := []dedupKey{{fingerprint: f.Fingerprint}}
	if len(f.CWE) > 0 {
		keys = append(keys, dedupKey{path: f.Location.Path, line: f.Location.StartLine, cwes: cweSet(f.CWE)})
	}
	return keys
}

// group is a set of findings reporting one issue.
type group struct {
	members []model.Finding // survivor first
	status  model.Status
	inputs  map[string]bool
	keys    map[dedupKey]bool
}

func (g *group) accepts(f model.Finding) bool {
	if f.Status != g.status || g.inputs[f.Source.Name] {
		return false
	}
	return slices.ContainsFunc(dedupKeys(f), func(k dedupKey) bool { return g.keys[k] })
}

func (g *group) add(f model.Finding) {
	g.members = append(g.members, f)
	g.inputs[f.Source.Name] = true
	for _, k := range dedupKeys(f) {
		g.keys[k] = true
	}
}

// DedupTools merges tool findings from different inputs that report the
// same issue (design §9 step 9). Findings are taken strongest first
// (severity, then input name, then canonical order) and each joins the
// first group that has the same status, no finding from its input yet, and
// a key in common; otherwise it starts a group. So a group never merges two
// findings of one input, never hides a new finding under an excused one,
// and its first member is its survivor. The others become the survivor's
// merged sources. The result is in canonical order and does not depend on
// the order of its input.
func DedupTools(fs []model.Finding) []model.Finding {
	order := slices.Clone(fs)
	slices.SortStableFunc(order, func(a, b model.Finding) int {
		switch {
		case beats(a, b):
			return -1
		case beats(b, a):
			return 1
		default:
			return 0
		}
	})

	var groups []*group
	var out []model.Finding
	for _, f := range order {
		if f.Source.Kind != model.SourceTool {
			out = append(out, f)
			continue
		}
		i := slices.IndexFunc(groups, func(g *group) bool { return g.accepts(f) })
		if i < 0 {
			groups = append(groups, &group{status: f.Status, inputs: map[string]bool{}, keys: map[dedupKey]bool{}})
			i = len(groups) - 1
		}
		groups[i].add(f)
	}
	for _, g := range groups {
		out = append(out, g.survivor())
	}
	slices.SortStableFunc(out, compareCanonical)
	return out
}

// survivor returns the group's first member with the rest as its merged
// sources.
func (g *group) survivor() model.Finding {
	s := g.members[0]
	if len(g.members) == 1 {
		return s
	}
	merged := slices.Clone(s.MergedSources)
	for _, f := range g.members[1:] {
		merged = append(merged, model.MergedSource{
			Input: f.Source.Name, RuleID: f.RuleID, Fingerprint: f.Fingerprint, Severity: f.Severity,
		})
		merged = append(merged, f.MergedSources...)
	}
	slices.SortFunc(merged, func(a, b model.MergedSource) int {
		return cmp.Or(cmp.Compare(a.Input, b.Input), cmp.Compare(a.RuleID, b.RuleID), cmp.Compare(a.Fingerprint, b.Fingerprint))
	})
	s.MergedSources = merged
	return s
}

// beats reports whether a is preferred to b as a survivor: higher
// severity, then the alphabetically first input, then canonical order.
func beats(a, b model.Finding) bool {
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
