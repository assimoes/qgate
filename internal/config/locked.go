package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// profilePrefix starts locked keys that address a field of every resolved
// profile rather than a top-level field.
const profilePrefix = "profile."

// LockedKey is a config path the org config forbids later layers to change.
type LockedKey struct {
	Key   string // dotted path into the resolved config
	Value any    // the value this run uses
	SetBy string // the layer that set it: the org config or builtin defaults
}

// formatValue renders a config value compactly for messages.
func formatValue(v any) string {
	data, err := json.Marshal(v)
	if err != nil { // only non-JSON numbers such as YAML's .nan
		return fmt.Sprint(v)
	}
	return string(data)
}

// FormatValue renders the locked value compactly for display.
func (k LockedKey) FormatValue() string { return formatValue(k.Value) }

// lockedKeys reads the org config's locked list and checks every key names
// a path in the resolved config.
func lockedKeys(sch *compiledSchemas, org layer) ([]string, Errors) {
	raw, _ := org.doc["locked"].([]any)
	var keys []string
	var errs Errors
	for i, r := range raw {
		key, _ := r.(string)
		if !sch.resolvedPathExists(strings.Split(key, ".")) {
			errs = append(errs, errorf(org.source, path{"locked"}.index(i).String(), "%q is not a config path", key))
			continue
		}
		keys = append(keys, key)
	}
	return keys, errs
}

// resolvedPathExists reports whether keys address a field of the resolved
// config schema.
func (s *compiledSchemas) resolvedPathExists(keys []string) bool {
	n := s.def("resolved")
	for _, k := range keys {
		if n = s.child(n, k); n == nil {
			return false
		}
	}
	return true
}

// checkRepoLocks reports every locked key the repo layer changes. A change
// is any difference that removing the locked fields from the repo layer
// would undo: for a top-level key, the merged value; for a profile key,
// that field of every profile, selected or not. Restating the locked value,
// or inheriting it through extends, is not a change.
func checkRepoLocks(keys []string, ls layers, repo layer) Errors {
	stripped := make(layers, len(ls))
	copy(stripped, ls)
	stripped[len(stripped)-1] = layer{source: repo.source, doc: stripLocked(repo.doc, keys)}
	full, base := ls.merge(), stripped.merge()

	var errs Errors
	for _, key := range keys {
		field, isProfile := strings.CutPrefix(key, profilePrefix)
		if !isProfile {
			want, _ := lookup(base, key)
			got, _ := lookup(full, key)
			if !reflect.DeepEqual(want, got) {
				errs = append(errs, lockedError(repo.source, key, want))
			}
			continue
		}
		errs = append(errs, checkProfileLock(field, full, base, repo)...)
	}
	return dedupe(errs)
}

// checkProfileLock compares one profile field across every profile, and
// blames the repo profile in the chain that sets it.
func checkProfileLock(field string, full, base map[string]any, repo layer) Errors {
	fullProfiles, _ := full["profiles"].(map[string]any)
	baseProfiles, _ := base["profiles"].(map[string]any)
	repoProfiles, _ := repo.doc["profiles"].(map[string]any)
	quiet := blameSource(repo.source) // broken chains are reported by checkProfiles

	var errs Errors
	for _, stage := range sortedKeys(fullProfiles) {
		got, gerrs := resolveProfile(fullProfiles, stage, quiet)
		want, werrs := resolveProfile(baseProfiles, stage, quiet)
		if len(gerrs) > 0 || len(werrs) > 0 {
			continue
		}
		g, _ := lookup(got, field)
		w, _ := lookup(want, field)
		if reflect.DeepEqual(g, w) {
			continue
		}
		chain, _ := profileChain(fullProfiles, stage, quiet)
		for _, name := range chain {
			rp, _ := repoProfiles[name].(map[string]any)
			if _, sets := lookup(rp, field); sets {
				errs = append(errs, lockedError(repo.source, "profiles."+name+"."+field, w))
				break
			}
		}
	}
	return errs
}

// stripLocked returns doc without the locked fields: top-level paths
// directly, profile fields from every profile.
func stripLocked(doc map[string]any, keys []string) map[string]any {
	out := deepCopy(doc).(map[string]any)
	profiles, _ := out["profiles"].(map[string]any)
	for _, key := range keys {
		if field, ok := strings.CutPrefix(key, profilePrefix); ok {
			for _, p := range profiles {
				if m, ok := p.(map[string]any); ok {
					deletePath(m, field)
				}
			}
			continue
		}
		deletePath(out, key)
	}
	return out
}

// checkSetLocks reports locked keys --set changed in the resolved config.
func checkSetLocks(keys []string, before, after map[string]any) Errors {
	var errs Errors
	for _, key := range keys {
		want, _ := lookup(before, key)
		got, _ := lookup(after, key)
		if !reflect.DeepEqual(want, got) {
			errs = append(errs, lockedError(setSource, key, want))
		}
	}
	return errs
}

func lockedError(source, at string, want any) Error {
	if want == nil {
		return errorf(source, at, "locked by the org config; it must stay unset")
	}
	return errorf(source, at, "locked by the org config; it must stay %s", formatValue(want))
}

// describeLocked lists the locked keys with the values this run uses and
// whether the org config or the builtin defaults set them.
func describeLocked(keys []string, resolved map[string]any, org layer) []LockedKey {
	out := make([]LockedKey, 0, len(keys))
	for _, key := range keys {
		v, _ := lookup(resolved, key)
		setBy := builtinSource
		if orgSets(org.doc, key) {
			setBy = org.source
		}
		out = append(out, LockedKey{Key: key, Value: v, SetBy: setBy})
	}
	return out
}

// orgSets reports whether the org config itself sets key: the path for a
// top-level key, the field in any of its profiles for a profile key.
func orgSets(doc map[string]any, key string) bool {
	field, isProfile := strings.CutPrefix(key, profilePrefix)
	if !isProfile {
		_, ok := lookup(doc, key)
		return ok
	}
	profiles, _ := doc["profiles"].(map[string]any)
	for _, p := range profiles {
		if m, ok := p.(map[string]any); ok {
			if _, sets := lookup(m, field); sets {
				return true
			}
		}
	}
	return false
}

// lookup returns the value at a dotted path, if present.
func lookup(doc map[string]any, key string) (any, bool) {
	var cur any = doc
	for _, k := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// deletePath removes the value at a dotted path, if present.
func deletePath(doc map[string]any, key string) {
	keys := strings.Split(key, ".")
	cur := doc
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
	delete(cur, keys[len(keys)-1])
}
