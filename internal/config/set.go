package config

import (
	"strings"
)

const setSource = "--set"

// applySets applies "dotted.path=value" overrides to the resolved document.
// Values are parsed as YAML, so `80` is a number and `[a, b]` a list.
func applySets(doc map[string]any, sets []string) Errors {
	var errs Errors
	for _, s := range sets {
		key, raw, ok := strings.Cut(s, "=")
		if !ok || key == "" {
			errs = append(errs, errorf(setSource, "", "%q is not of the form path=value", s))
			continue
		}
		if key == "stage" {
			errs = append(errs, errorf(setSource, key, "the stage selects the profile; use --stage instead"))
			continue
		}
		val, err := parseScalar(raw)
		if err != nil {
			errs = append(errs, errorf(setSource, key, "invalid value: %v", err))
			continue
		}
		if err := setPath(doc, strings.Split(key, "."), val); err != nil {
			errs = append(errs, *err)
		}
	}
	return errs
}

func setPath(doc map[string]any, keys []string, val any) *Error {
	cur := doc
	for i, k := range keys[:len(keys)-1] {
		next, exists := cur[k]
		if !exists || next == nil {
			m := map[string]any{}
			cur[k] = m
			cur = m
			continue
		}
		m, ok := next.(map[string]any)
		if !ok {
			e := errorf(setSource, strings.Join(keys[:i+1], "."), "is not a mapping, cannot set %s", strings.Join(keys, "."))
			return &e
		}
		cur = m
	}
	cur[keys[len(keys)-1]] = val
	return nil
}
