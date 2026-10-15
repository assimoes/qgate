package config

// Merge rules between layers (design §5.1): maps deep-merge, scalars and
// lists replace, except for the top-level lists below. Both merging and
// working out which layer set a list element follow listRuleFor.
var (
	// keyedLists merge element by element, matching elements by a key field.
	keyedLists = map[string]string{"inputs": "name", "review_rules": "id"}
	// appendedLists concatenate, later layers after earlier ones.
	appendedLists = map[string]bool{"suppressions": true}
)

// listRule is how a top-level list merges across layers.
type listRule int

const (
	replaceList  listRule = iota // a later layer's list replaces the earlier one
	keyedList                    // elements merge by key; new keys are appended
	appendedList                 // later layers' elements follow earlier ones
)

// listRuleFor returns the merge rule for the top-level list name, and the
// key field for a keyed list.
func listRuleFor(name string) (listRule, string) {
	switch {
	case keyedLists[name] != "":
		return keyedList, keyedLists[name]
	case appendedLists[name]:
		return appendedList, ""
	default:
		return replaceList, ""
	}
}

// mergeLayers returns base with over applied on top. Neither is modified.
func mergeLayers(base, over map[string]any) map[string]any {
	out := deepCopy(base).(map[string]any)
	for _, k := range sortedKeys(over) {
		v := deepCopy(over[k])
		switch rule, key := listRuleFor(k); rule {
		case keyedList:
			out[k] = mergeKeyed(out[k], v, key)
		case appendedList:
			prev, _ := out[k].([]any)
			next, _ := v.([]any)
			out[k] = append(prev, next...)
		default:
			out[k] = deepMerge(out[k], v)
		}
	}
	return out
}

// deepMerge merges maps key by key; anything else in over replaces base.
func deepMerge(base, over any) any {
	bm, ok1 := base.(map[string]any)
	om, ok2 := over.(map[string]any)
	if !ok1 || !ok2 {
		return over
	}
	for _, k := range sortedKeys(om) {
		bm[k] = deepMerge(bm[k], om[k])
	}
	return bm
}

// mergeKeyed merges over's elements into base by their key field: a matching
// element is deep-merged in place, a new one is appended.
func mergeKeyed(base, over any, key string) any {
	bl, _ := base.([]any)
	ol, ok := over.([]any)
	if !ok {
		return over
	}
	index := make(map[any]int, len(bl))
	for i, e := range bl {
		if m, ok := e.(map[string]any); ok {
			index[m[key]] = i
		}
	}
	for _, e := range ol {
		m, ok := e.(map[string]any)
		if i, found := index[m[key]]; ok && found {
			bl[i] = deepMerge(bl[i], m)
			continue
		}
		bl = append(bl, e)
	}
	return bl
}
