package config

import (
	"slices"
	"strings"
)

// profileDefaults are the values every resolved profile starts from, so a
// profile only states what differs.
func profileDefaults() map[string]any {
	return map[string]any{
		"enforcement":    "advisory",
		"scope":          "diff",
		"agents":         []any{},
		"on_input_error": "warn",
		"on_agent_error": "warn",
		"gate":           map[string]any{},
	}
}

// resolveProfile returns the profile a stage selects, with its extends chain
// merged root-first onto the defaults.
func resolveProfile(profiles map[string]any, stage string, blame blame) (map[string]any, Errors) {
	if _, ok := profiles[stage]; !ok {
		return nil, Errors{blame(nil, "unknown stage %q: no profile with that name (profiles: %s)",
			stage, strings.Join(sortedKeys(profiles), ", "))}
	}

	names, errs := profileChain(profiles, stage, blame)
	if len(errs) > 0 {
		return nil, errs
	}
	resolved := profileDefaults()
	for _, name := range slices.Backward(names) {
		p := deepCopy(profiles[name]).(map[string]any)
		delete(p, "extends")
		resolved = deepMerge(resolved, p).(map[string]any)
	}
	return resolved, nil
}

// profileChain returns the names from stage's profile to the root of its
// extends chain.
func profileChain(profiles map[string]any, stage string, blame blame) ([]string, Errors) {
	var names []string
	for name := stage; name != ""; {
		if slices.Contains(names, name) {
			return nil, Errors{blame(path{"profiles", name, "extends"},
				"extends cycle: %s", strings.Join(append(names, name), " → "))}
		}
		p, ok := profiles[name].(map[string]any)
		if !ok {
			return nil, Errors{blame(path{"profiles", names[len(names)-1], "extends"},
				"extends unknown profile %q", name)}
		}
		names = append(names, name)
		name, _ = p["extends"].(string)
	}
	return names, nil
}

// checkProfiles reports every profile whose extends chain is broken, not
// only the selected one, so a typo in an unused profile is still caught.
func checkProfiles(profiles map[string]any, blame blame) Errors {
	var errs Errors
	for _, name := range sortedKeys(profiles) {
		if _, e := resolveProfile(profiles, name, blame); len(e) > 0 {
			errs = append(errs, e...)
		}
	}
	return dedupe(errs)
}
