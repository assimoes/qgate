package config

import (
	"regexp"
)

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// interpolate expands ${VAR} in the fields the schema marks
// x-qgate-interpolate and nowhere else, so that text such as a review rule's
// description is never rewritten. An unset variable is a config error.
func interpolate(s *compiledSchemas, layer map[string]any, env map[string]string, file string) Errors {
	var errs Errors
	var walk func(v any, n schemaNode, p path) any
	walk = func(v any, n schemaNode, p path) any {
		if n == nil {
			return v
		}
		switch t := v.(type) {
		case map[string]any:
			for _, k := range sortedKeys(t) {
				t[k] = walk(t[k], s.child(n, k), p.key(k))
			}
			return t
		case []any:
			it := s.items(n)
			for i := range t {
				t[i] = walk(t[i], it, p.index(i))
			}
			return t
		case string:
			if !s.interpolatable(n) {
				return t
			}
			return envRef.ReplaceAllStringFunc(t, func(ref string) string {
				name := envRef.FindStringSubmatch(ref)[1]
				val, ok := env[name]
				if !ok {
					errs = append(errs, errorf(file, p.String(), "environment variable %s is not set", name))
				}
				return val
			})
		default:
			return v
		}
	}
	walk(layer, s.def("layer"), nil)
	return errs
}
