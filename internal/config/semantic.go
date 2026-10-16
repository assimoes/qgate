package config

import (
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"qgate/internal/model"
	"qgate/internal/prompts"
)

// builtinCategories are always known; custom categories must be declared in
// scoring.category_weights.
var builtinCategories = []string{"security", "quality", "maintainability", "testing", "dependencies"}

// checkDuplicates reports keyed-list elements that repeat a key within one
// layer; across layers the same key merges instead.
func checkDuplicates(layer map[string]any, file string) Errors {
	var errs Errors
	for _, list := range sortedKeys(keyedLists) {
		key := keyedLists[list]
		items, _ := layer[list].([]any)
		first := map[any]int{}
		for i, it := range items {
			m, _ := it.(map[string]any)
			k, ok := m[key]
			if !ok {
				continue // a missing key is reported as a required property
			}
			if j, dup := first[k]; dup {
				errs = append(errs, errorf(file, path{list}.index(i).key(key).String(),
					"duplicate %s %v (first defined at %s)", key, k, path{list}.index(j)))
				continue
			}
			first[k] = i
		}
	}
	return errs
}

// checkSemantics reports references to things that do not exist, invalid
// globs and agent kinds that contradict their builtin prompt.
func checkSemantics(doc map[string]any, blame blame) Errors {
	c := checker{doc: doc, blame: blame}
	c.categories()
	c.categoryWeights()
	c.providerRefs()
	c.agentRefs()
	c.agentKinds()
	c.globs()
	sort.SliceStable(c.errs, func(i, j int) bool { return c.errs[i].Path < c.errs[j].Path })
	return c.errs
}

type checker struct {
	doc   map[string]any
	blame blame
	errs  Errors
}

// profileAt is a profile and where it sits in the document.
type profileAt struct {
	p path
	m map[string]any
}

// profiles returns every profile in the document with its path: all of
// profiles.<name> in a layered document, or the single resolved profile.
func (c *checker) profiles() []profileAt {
	var out []profileAt
	all := c.mapAt("profiles")
	for _, name := range sortedKeys(all) {
		m, _ := all[name].(map[string]any)
		out = append(out, profileAt{path{"profiles", name}, m})
	}
	if m := c.mapAt("profile"); m != nil {
		out = append(out, profileAt{path{"profile"}, m})
	}
	return out
}

func (c *checker) fail(p path, format string, args ...any) {
	c.errs = append(c.errs, c.blame(p, format, args...))
}

func (c *checker) mapAt(keys ...string) map[string]any {
	var cur any = c.doc
	for _, k := range keys {
		m, _ := cur.(map[string]any)
		cur = m[k]
	}
	m, _ := cur.(map[string]any)
	return m
}

// eachAgent calls fn for every agent, in name order.
func (c *checker) eachAgent(fn func(name string, agent map[string]any)) {
	agents := c.mapAt("agents")
	for _, name := range sortedKeys(agents) {
		m, _ := agents[name].(map[string]any)
		fn(name, m)
	}
}

func listAt(m map[string]any, key string) []any {
	l, _ := m[key].([]any)
	return l
}

func (c *checker) categories() {
	known := map[string]bool{}
	for _, cat := range builtinCategories {
		known[cat] = true
	}
	for k := range c.mapAt("scoring", "category_weights") {
		known[k] = true
	}
	check := func(p path, v any) {
		if s, ok := v.(string); ok && !known[s] {
			c.fail(p, "unknown category %q; declare custom categories in scoring.category_weights", s)
		}
	}

	for i, in := range listAt(c.doc, "inputs") {
		m, _ := in.(map[string]any)
		p := path{"inputs"}.index(i)
		check(p.key("category_default"), m["category_default"])
		for j, a := range listAt(m, "assesses") {
			check(p.key("assesses").index(j), a)
		}
	}
	for i, cm := range listAt(c.doc, "category_mappings") {
		m, _ := cm.(map[string]any)
		check(path{"category_mappings"}.index(i).key("category"), m["category"])
	}
	c.eachAgent(func(name string, m map[string]any) {
		for j, cat := range listAt(m, "categories") {
			check(path{"agents", name, "categories"}.index(j), cat)
		}
	})
	for i, rr := range listAt(c.doc, "review_rules") {
		m, _ := rr.(map[string]any)
		check(path{"review_rules"}.index(i).key("category"), m["category"])
	}
	for _, prof := range c.profiles() {
		gate, _ := prof.m["gate"].(map[string]any)
		catMin, _ := gate["category_min"].(map[string]any)
		for _, cat := range sortedKeys(catMin) {
			check(prof.p.key("gate").key("category_min").key(cat), cat)
		}
		for j, cat := range listAt(gate, "require_assessed") {
			check(prof.p.key("gate").key("require_assessed").index(j), cat)
		}
	}
}

// categoryWeights rejects weights that are all zero: the overall score
// would never exist, however much was assessed.
func (c *checker) categoryWeights() {
	weights := c.mapAt("scoring", "category_weights")
	if len(weights) == 0 {
		return
	}
	for _, w := range weights {
		if n, ok := w.(int64); ok && n > 0 {
			return
		}
	}
	c.fail(path{"scoring", "category_weights"}, "at least one category weight must be positive")
}

func (c *checker) providerRefs() {
	providers := c.mapAt("providers")
	for _, name := range sortedKeys(providers) {
		m, _ := providers[name].(map[string]any)
		if fb, ok := m["fallback"].(string); ok {
			target, _, _ := strings.Cut(fb, "/")
			if _, found := providers[target]; !found {
				c.fail(path{"providers", name, "fallback"}, "unknown provider %q", target)
			}
		}
	}
	c.eachAgent(func(name string, m map[string]any) {
		if p, ok := m["provider"].(string); ok {
			if _, found := providers[p]; !found {
				c.fail(path{"agents", name, "provider"}, "unknown provider %q", p)
			}
		}
	})
}

func (c *checker) agentRefs() {
	agents := c.mapAt("agents")
	for _, prof := range c.profiles() {
		for j, a := range listAt(prof.m, "agents") {
			if s, _ := a.(string); agents[s] == nil {
				c.fail(prof.p.key("agents").index(j), "unknown agent %q", s)
			}
		}
	}
	for i, rr := range listAt(c.doc, "review_rules") {
		m, _ := rr.(map[string]any)
		for j, a := range listAt(m, "agents") {
			p := path{"review_rules"}.index(i).key("agents").index(j)
			s, _ := a.(string)
			agent, _ := agents[s].(map[string]any)
			switch {
			case agent == nil:
				c.fail(p, "unknown agent %q", s)
			case agent["kind"] != string(model.AgentKindReviewer):
				c.fail(p, "agent %q is a %v; review rules are given to reviewers", s, agent["kind"])
			}
		}
	}
}

func (c *checker) agentKinds() {
	c.eachAgent(func(name string, m map[string]any) {
		prompt, _ := m["prompt"].(map[string]any)
		base, _ := prompt["base"].(string)
		p := path{"agents", name, "prompt", "base"}
		ref, isBuiltin, err := prompts.ParseBuiltin(base)
		if !isBuiltin {
			return
		}
		if err != nil {
			c.fail(p, "%v", err)
			return
		}
		kind, err := prompts.Kind(ref)
		if err != nil {
			c.fail(p, "%v", err)
			return
		}
		if m["kind"] != string(kind) {
			c.fail(p, "builtin prompt %s is for a %s, but agent %q is a %v", ref.Name, kind, name, m["kind"])
		}
	})
}

func (c *checker) globs() {
	check := func(p path, v any) {
		if s, ok := v.(string); ok && !doublestar.ValidatePattern(s) {
			c.fail(p, "invalid glob %q", s)
		}
	}
	checkList := func(p path, l []any) {
		for i, v := range l {
			check(p.index(i), v)
		}
	}
	project := c.mapAt("project")
	checkList(path{"project", "include"}, listAt(project, "include"))
	checkList(path{"project", "exclude"}, listAt(project, "exclude"))
	checkList(path{"llm", "exclude_paths"}, listAt(c.mapAt("llm"), "exclude_paths"))

	c.eachAgent(func(name string, m map[string]any) {
		checkList(path{"agents", name, "priority_paths"}, listAt(m, "priority_paths"))
	})
	for _, list := range []string{"review_rules", "suppressions"} {
		for i, it := range listAt(c.doc, list) {
			m, _ := it.(map[string]any)
			checkList(path{list}.index(i).key("paths"), listAt(m, "paths"))
			if list == "suppressions" {
				check(path{list}.index(i).key("rule"), m["rule"])
			}
		}
	}
	for i, cm := range listAt(c.doc, "category_mappings") {
		m, _ := cm.(map[string]any)
		match, _ := m["match"].(map[string]any)
		for _, k := range []string{"rule_id", "path", "cwe"} {
			check(path{"category_mappings"}.index(i).key("match").key(k), match[k])
		}
	}
}
