// Package glob matches the two kinds of pattern qgate's config uses: path
// globs, where * stops at / and ** crosses directories, and rule-ID globs,
// where * matches any run of characters, slashes included, because rule
// IDs such as codeql:go/sql-injection contain slashes without being paths.
package glob

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Path reports whether a repo-relative path matches a path glob.
func Path(pattern, p string) bool {
	ok, _ := doublestar.Match(pattern, p)
	return ok
}

// ValidPath reports whether pattern is a well-formed path glob.
func ValidPath(pattern string) bool { return doublestar.ValidatePattern(pattern) }

// slash stands in for "/" so path.Match treats rule IDs as one segment.
var slash = strings.NewReplacer("/", "￿")

// Rule reports whether a rule ID (or any other identifier, such as a CWE)
// matches a rule glob: * matches any run of characters including /, ?
// matches one character, and [...] a character class.
func Rule(pattern, id string) bool {
	ok, err := path.Match(slash.Replace(pattern), slash.Replace(id))
	return err == nil && ok
}

// ValidRule reports whether pattern is a well-formed rule glob.
func ValidRule(pattern string) bool {
	_, err := path.Match(slash.Replace(pattern), "")
	return err == nil
}
