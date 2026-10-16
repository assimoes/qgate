// Package report renders a run's report. Each output format is a Format,
// registered by name; adding a format means adding a Format, not editing
// existing ones. The console is not a Format: it always writes to stdout.
package report

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/assimoes/qgate/internal/model"
)

// Format renders a report as the contents of one file.
type Format interface {
	Name() string
	Render(r *model.Report) ([]byte, error)
}

// Registry finds the format for a name.
type Registry struct {
	byName map[string]Format
}

// NewRegistry registers the given formats by name.
func NewRegistry(formats ...Format) *Registry {
	r := &Registry{byName: make(map[string]Format, len(formats))}
	for _, f := range formats {
		r.byName[f.Name()] = f
	}
	return r
}

// Lookup returns the format called name.
func (r *Registry) Lookup(name string) (Format, bool) {
	f, ok := r.byName[name]
	return f, ok
}

// Names lists the registered formats.
func (r *Registry) Names() []string { return slices.Sorted(maps.Keys(r.byName)) }

// WriteFile writes data to path atomically, creating parent directories:
// a reader never sees a half-written report.
func WriteFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".qgate-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// topFindings returns the n most severe new findings, most severe first,
// ties in report order (which is canonical).
func topFindings(r *model.Report, n int) []model.Finding {
	var out []model.Finding
	for _, f := range r.Findings {
		if f.Status == model.StatusNew {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b model.Finding) int { return int(b.Severity) - int(a.Severity) })
	return out[:min(n, len(out))]
}

// location renders a finding's location as path or path:line.
func location(f model.Finding) string {
	if f.Location.StartLine == 0 {
		return f.Location.Path
	}
	return fmt.Sprintf("%s:%d", f.Location.Path, f.Location.StartLine)
}
