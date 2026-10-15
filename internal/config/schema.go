package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"qgate/schemas"
)

const configSchemaURL = "config.schema.json"

type compiledSchemas struct {
	layer    *jsonschema.Schema
	resolved *jsonschema.Schema
	raw      map[string]any // the schema document, for x-qgate-interpolate lookups
}

// loadSchemas compiles the embedded config schema once. The result is
// immutable after the first call.
var loadSchemas = sync.OnceValues(func() (*compiledSchemas, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.Config))
	if err != nil {
		return nil, fmt.Errorf("parse config schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(configSchemaURL, doc); err != nil {
		return nil, fmt.Errorf("add config schema: %w", err)
	}
	layer, err := c.Compile(configSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile config schema: %w", err)
	}
	resolved, err := c.Compile(configSchemaURL + "#/$defs/resolved")
	if err != nil {
		return nil, fmt.Errorf("compile resolved config schema: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(schemas.Config, &raw); err != nil {
		return nil, fmt.Errorf("parse config schema: %w", err)
	}
	return &compiledSchemas{layer: layer, resolved: resolved, raw: raw}, nil
})

var printer = message.NewPrinter(language.English)

// validateSchema checks doc against sch and reports each violation with the
// file and dotted path it concerns.
func validateSchema(sch *jsonschema.Schema, doc map[string]any, file string) (Errors, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	err = sch.Validate(inst)
	var ve *jsonschema.ValidationError
	switch {
	case err == nil:
		return nil, nil
	case !errors.As(err, &ve):
		return nil, fmt.Errorf("validate config: %w", err)
	}

	var errs Errors
	for _, leaf := range leaves(ve) {
		errs = append(errs, Error{
			File:    file,
			Path:    instancePath(doc, leaf.InstanceLocation),
			Message: leaf.ErrorKind.LocalizedString(printer),
		})
	}
	sort.SliceStable(errs, func(i, j int) bool { return errs[i].Path < errs[j].Path })
	return dedupe(explainUnevaluated(errs)), nil
}

// falseSchema is how the validator reports a property rejected by
// unevaluatedProperties: false (used by profiles, which extend shared fields).
const falseSchema = "false schema"

// explainUnevaluated rewrites "false schema" into "unknown property", and
// drops it where the same path already has a more specific error: a property
// that fails its own schema also counts as unevaluated.
func explainUnevaluated(errs Errors) Errors {
	specific := map[string]bool{}
	for _, e := range errs {
		if e.Message != falseSchema {
			specific[e.Path] = true
		}
	}
	out := errs[:0]
	for _, e := range errs {
		if e.Message == falseSchema {
			if specific[e.Path] {
				continue
			}
			e.Message = "unknown property"
		}
		out = append(out, e)
	}
	return out
}

// leaves returns the most specific causes of a validation error.
func leaves(ve *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(ve.Causes) == 0 {
		return []*jsonschema.ValidationError{ve}
	}
	var out []*jsonschema.ValidationError
	for _, c := range ve.Causes {
		out = append(out, leaves(c)...)
	}
	return out
}

// instancePath renders a JSON pointer as a dotted path, using the document
// to tell list indexes from map keys.
func instancePath(doc any, tokens []string) string {
	var p path
	cur := doc
	for _, tok := range tokens {
		switch t := cur.(type) {
		case []any:
			var i int
			fmt.Sscanf(tok, "%d", &i)
			p = p.index(i)
			if i >= 0 && i < len(t) {
				cur = t[i]
			}
		case map[string]any:
			p = p.key(tok)
			cur = t[tok]
		default:
			p = p.key(tok)
		}
	}
	return p.String()
}

func dedupe(errs Errors) Errors {
	seen := make(map[string]bool, len(errs))
	out := errs[:0]
	for _, e := range errs {
		k := e.Path + "\x00" + e.Message
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

// schemaNode is one node of the raw schema document.
type schemaNode map[string]any

// def returns the named $defs entry of the raw schema.
func (s *compiledSchemas) def(name string) schemaNode {
	defs, _ := s.raw["$defs"].(map[string]any)
	n, _ := defs[name].(map[string]any)
	return n
}

// deref follows a local "#/$defs/x" $ref.
func (s *compiledSchemas) deref(n schemaNode) schemaNode {
	ref, ok := n["$ref"].(string)
	if !ok {
		return nil
	}
	return s.def(strings.TrimPrefix(ref, "#/$defs/"))
}

// child returns the schema for the value at key of an object matched by n.
func (s *compiledSchemas) child(n schemaNode, key string) schemaNode {
	for cur := n; cur != nil; cur = s.deref(cur) {
		if props, ok := cur["properties"].(map[string]any); ok {
			if c, ok := props[key].(map[string]any); ok {
				return c
			}
		}
		if ap, ok := cur["additionalProperties"].(map[string]any); ok {
			return ap
		}
	}
	return nil
}

// items returns the schema for the elements of an array matched by n.
func (s *compiledSchemas) items(n schemaNode) schemaNode {
	for cur := n; cur != nil; cur = s.deref(cur) {
		if it, ok := cur["items"].(map[string]any); ok {
			return it
		}
	}
	return nil
}

// interpolatable reports whether n is marked x-qgate-interpolate.
func (s *compiledSchemas) interpolatable(n schemaNode) bool {
	for cur := n; cur != nil; cur = s.deref(cur) {
		if v, _ := cur["x-qgate-interpolate"].(bool); v {
			return true
		}
	}
	return false
}
