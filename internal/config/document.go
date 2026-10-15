package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// A document is a config decoded into plain values: map[string]any, []any,
// string, int64, float64, bool and nil. Every layer is a document until the
// resolved config is decoded into typed structs.

// parseYAML decodes data into a document. It works from the YAML node tree so
// that scalars keep their written form (an unquoted date stays a string) and
// duplicate keys are rejected.
func parseYAML(data []byte) (map[string]any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if root.Kind == 0 { // empty file
		return map[string]any{}, nil
	}
	v, err := nodeValue(root.Content[0])
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("top level must be a mapping")
	}
	return m, nil
}

func nodeValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return nodeValue(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("line %d: mapping keys must be strings", k.Line)
			}
			if _, dup := m[k.Value]; dup {
				return nil, fmt.Errorf("line %d: duplicate key %q", k.Line, k.Value)
			}
			val, err := nodeValue(v)
			if err != nil {
				return nil, err
			}
			m[k.Value] = val
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, len(n.Content))
		for i, c := range n.Content {
			v, err := nodeValue(c)
			if err != nil {
				return nil, err
			}
			s[i] = v
		}
		return s, nil
	case yaml.ScalarNode:
		return scalarValue(n)
	default:
		return nil, fmt.Errorf("line %d: unsupported YAML node", n.Line)
	}
}

func scalarValue(n *yaml.Node) (any, error) {
	switch n.ShortTag() {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		err := n.Decode(&b)
		return b, err
	case "!!int":
		var i int64
		err := n.Decode(&i)
		return i, err
	case "!!float":
		var f float64
		err := n.Decode(&f)
		return f, err
	default: // !!str, !!timestamp, !!binary: keep the text as written
		return n.Value, nil
	}
}

// parseScalar decodes a single YAML value, as given to --set.
func parseScalar(s string) (any, error) {
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(s), &n); err != nil {
		return nil, err
	}
	if n.Kind == 0 {
		return nil, nil
	}
	return nodeValue(n.Content[0])
}

// deepCopy copies a document value so layers never alias each other.
func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, x := range t {
			m[k] = deepCopy(x)
		}
		return m
	case []any:
		s := make([]any, len(t))
		for i, x := range t {
			s[i] = deepCopy(x)
		}
		return s
	default:
		return v
	}
}

// sortedKeys returns m's keys in order, so every walk is deterministic.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// path is a location in a document, rendered as "inputs[0].format".
type path []string

func (p path) key(k string) path { return append(p[:len(p):len(p)], k) }
func (p path) index(i int) path  { return append(p[:len(p):len(p)], "["+strconv.Itoa(i)+"]") }

func (p path) String() string {
	var b strings.Builder
	for _, s := range p {
		if b.Len() > 0 && !strings.HasPrefix(s, "[") {
			b.WriteByte('.')
		}
		b.WriteString(s)
	}
	return b.String()
}
