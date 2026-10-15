package config

import (
	"encoding/json"
	"reflect"
	"strings"
)

// LockedKey is a config path the org config forbids later layers to change.
type LockedKey struct {
	Key   string // dotted path into the resolved config
	Value any    // the value every run uses
	SetBy string // the layer that set the value: the org config or builtin defaults
}

// FormatValue renders the locked value compactly for display.
func (k LockedKey) FormatValue() string {
	data, err := json.Marshal(k.Value)
	if err != nil {
		return "?"
	}
	return string(data)
}

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

// checkLocked reports every locked key whose value differs between before
// and after, naming the layer responsible.
func checkLocked(keys []string, before, after map[string]any, changedBy string) Errors {
	var errs Errors
	for _, key := range keys {
		want, _ := lookup(before, key)
		got, _ := lookup(after, key)
		if !reflect.DeepEqual(want, got) {
			errs = append(errs, errorf(changedBy, key, "locked by the org config; it must stay %s",
				LockedKey{Value: want}.FormatValue()))
		}
	}
	return errs
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
