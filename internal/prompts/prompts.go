// Package prompts holds qgate's builtin, versioned agent prompts. Released
// versions are immutable; a change ships as a new version.
package prompts

import (
	"fmt"
	"strconv"
	"strings"
)

// BuiltinScheme prefixes references to builtin prompts.
const BuiltinScheme = "builtin://"

// builtin describes one builtin prompt.
type builtin struct {
	kind     string // agent kind it is written for
	versions int    // versions 1..versions exist
}

// builtins is the catalog of builtin prompts. Adding a prompt or a version
// means adding an entry here and its text, never editing a released one.
var builtins = map[string]builtin{
	"security-reviewer": {kind: "reviewer", versions: 1},
	"quality-reviewer":  {kind: "reviewer", versions: 1},
	"triage":            {kind: "triager", versions: 1},
}

// Ref identifies a builtin prompt version, e.g. builtin://triage@1.
type Ref struct {
	Name    string
	Version int
}

// ParseBuiltin parses a builtin:// reference. ok is false when base does not
// use the builtin scheme (it then names a team's own prompt file).
func ParseBuiltin(base string) (ref Ref, ok bool, err error) {
	rest, ok := strings.CutPrefix(base, BuiltinScheme)
	if !ok {
		return Ref{}, false, nil
	}
	name, ver, found := strings.Cut(rest, "@")
	if !found {
		return Ref{}, true, fmt.Errorf("builtin prompt %q must pin a version, e.g. %s%s@1", base, BuiltinScheme, rest)
	}
	v, err := strconv.Atoi(ver)
	if err != nil || v < 1 {
		return Ref{}, true, fmt.Errorf("builtin prompt %q has an invalid version", base)
	}
	return Ref{Name: name, Version: v}, true, nil
}

// Kind returns the agent kind a builtin prompt version is written for.
func Kind(ref Ref) (string, error) {
	b, ok := builtins[ref.Name]
	if !ok {
		return "", fmt.Errorf("unknown builtin prompt %q", ref.Name)
	}
	if ref.Version > b.versions {
		return "", fmt.Errorf("builtin prompt %s has no version %d (latest is %d)", ref.Name, ref.Version, b.versions)
	}
	return b.kind, nil
}
