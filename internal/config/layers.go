package config

import (
	"strconv"
	"strings"
)

// layer is one config document and where it came from.
type layer struct {
	source string // as shown in messages: "builtin defaults", a path or a URL
	doc    map[string]any
}

// layers are config layers in precedence order, lowest first.
type layers []layer

// merge folds every layer into one document.
func (ls layers) merge() map[string]any {
	merged := map[string]any{}
	for _, l := range ls {
		merged = mergeLayers(merged, l.doc)
	}
	return merged
}

// top is the highest-precedence layer's source, used for problems that are
// not tied to one path.
func (ls layers) top() string { return ls[len(ls)-1].source }

// origin returns the source of the highest-precedence layer that sets p in
// the merged document, so errors found after merging name the file to fix.
func (ls layers) origin(merged map[string]any, p path) string {
	for i := len(ls) - 1; i >= 0; i-- {
		if ls.sets(i, merged, p) {
			return ls[i].source
		}
	}
	return ls.top()
}

// sets reports whether layer i contributes the value at p in merged,
// following the merge rules: keyed lists match elements by key, appended
// lists by position after the earlier layers' elements, other lists and
// scalars by presence.
func (ls layers) sets(i int, merged map[string]any, p path) bool {
	var cur, own any = merged, ls[i].doc
	parent := ""
	for depth, tok := range p {
		if idx, ok := indexToken(tok); ok {
			ml, _ := cur.([]any)
			ol, _ := own.([]any)
			if idx >= len(ml) {
				return false
			}
			elem := ml[idx]
			rule, key := replaceList, ""
			if depth == 1 { // only top-level lists have special rules
				rule, key = listRuleFor(parent)
			}
			switch rule {
			case keyedList:
				m, _ := elem.(map[string]any)
				own = findByKey(ol, key, m[key])
			case appendedList:
				offset := 0
				for _, earlier := range ls[:i] {
					l, _ := earlier.doc[parent].([]any)
					offset += len(l)
				}
				own = nil
				if j := idx - offset; j >= 0 && j < len(ol) {
					own = ol[j]
				}
			default:
				own = nil
				if idx < len(ol) {
					own = ol[idx]
				}
			}
			if own == nil {
				return false
			}
			cur = elem
			continue
		}
		mm, _ := cur.(map[string]any)
		om, _ := own.(map[string]any)
		v, ok := om[tok]
		if !ok {
			return false
		}
		cur, own, parent = mm[tok], v, tok
	}
	return true
}

func indexToken(tok string) (int, bool) {
	inner, ok := strings.CutPrefix(tok, "[")
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(strings.TrimSuffix(inner, "]"))
	return i, err == nil
}

func findByKey(list []any, key string, want any) any {
	for _, e := range list {
		if m, ok := e.(map[string]any); ok && m[key] == want {
			return e
		}
	}
	return nil
}

// blame returns an Error for p in the merged document, naming the layer
// that set it.
func (ls layers) blame(merged map[string]any) blame {
	return func(p path, format string, args ...any) Error {
		return errorf(ls.origin(merged, p), p.String(), format, args...)
	}
}
