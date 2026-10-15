package config

import (
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
func resolveProfile(profiles map[string]any, stage, file string) (map[string]any, Errors) {
	if _, ok := profiles[stage]; !ok {
		return nil, Errors{errorf(file, "", "unknown stage %q: no profile with that name (profiles: %s)",
			stage, strings.Join(sortedKeys(profiles), ", "))}
	}

	// Walk from the selected profile to the root of its chain.
	var chain []map[string]any
	seen := map[string]bool{}
	for name := stage; name != ""; {
		if seen[name] {
			return nil, Errors{errorf(file, path{"profiles", name, "extends"}.String(),
				"extends cycle: %s", strings.Join(append(chainNames(chain), name), " → "))}
		}
		seen[name] = true
		p, ok := profiles[name].(map[string]any)
		if !ok {
			from := chainNames(chain)[len(chain)-1]
			return nil, Errors{errorf(file, path{"profiles", from, "extends"}.String(),
				"extends unknown profile %q", name)}
		}
		p = deepCopy(p).(map[string]any)
		p["$name"] = name
		chain = append(chain, p)
		name, _ = p["extends"].(string)
	}

	resolved := profileDefaults()
	for i := len(chain) - 1; i >= 0; i-- {
		p := chain[i]
		delete(p, "extends")
		delete(p, "$name")
		resolved = deepMerge(resolved, p).(map[string]any)
	}
	return resolved, nil
}

func chainNames(chain []map[string]any) []string {
	names := make([]string, len(chain))
	for i, p := range chain {
		names[i], _ = p["$name"].(string)
	}
	return names
}

// checkProfiles reports every profile whose extends chain is broken, not
// only the selected one, so a typo in an unused profile is still caught.
func checkProfiles(profiles map[string]any, file string) Errors {
	var errs Errors
	for _, name := range sortedKeys(profiles) {
		if _, e := resolveProfile(profiles, name, file); len(e) > 0 {
			errs = append(errs, e...)
		}
	}
	return dedupe(errs)
}
