package config

import (
	"fmt"
	"strings"
)

// Error is one problem with the configuration.
type Error struct {
	File    string // config file, "builtin defaults" or "--set"
	Path    string // dotted path into the document, e.g. "inputs[0].format"; empty for the whole document
	Message string
}

func (e Error) Error() string {
	var b strings.Builder
	if e.File != "" {
		b.WriteString(e.File)
		b.WriteString(": ")
	}
	if e.Path != "" {
		b.WriteString(e.Path)
		b.WriteString(": ")
	}
	b.WriteString(e.Message)
	return b.String()
}

// Errors is every problem found in one pass. Config errors are reported
// together so a team can fix them all at once.
type Errors []Error

func (es Errors) Error() string {
	lines := make([]string, len(es))
	for i, e := range es {
		lines[i] = e.Error()
	}
	return "invalid config:\n  " + strings.Join(lines, "\n  ")
}

// orNil returns es as an error, or nil when it is empty.
func (es Errors) orNil() error {
	if len(es) == 0 {
		return nil
	}
	return es
}

func errorf(file, path, format string, args ...any) Error {
	return Error{File: file, Path: path, Message: fmt.Sprintf(format, args...)}
}

// blame builds an Error for a path in a document, naming the source
// responsible for it.
type blame func(p path, format string, args ...any) Error

// blameSource blames a single, fixed source.
func blameSource(source string) blame {
	return func(p path, format string, args ...any) Error {
		return errorf(source, p.String(), format, args...)
	}
}
