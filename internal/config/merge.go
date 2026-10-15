package config

// Merge rules between layers (design §5.1): maps deep-merge, scalars and
// lists replace, except for the lists below.
var (
	// keyedLists merge element by element, matching elements by a key field.
	keyedLists = map[string]string{"inputs": "name", "review_rules": "id"}
	// appendedLists concatenate, later layers after earlier ones.
	appendedLists = map[string]bool{"suppressions": true}
)

// mergeLayers returns base with over applied on top. Neither is modified.
func mergeLayers(base, over map[string]any) map[string]any {
	out := deepCopy(base).(map[string]any)
	for _, k := range sortedKeys(over) {
		v := deepCopy(over[k])
		switch {
		case keyedLists[k] != "":
			out[k] = mergeKeyed(out[k], v, keyedLists[k])
		case appendedLists[k]:
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
