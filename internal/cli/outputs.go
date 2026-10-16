package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/report"
)

// formats are the report formats this build writes. Markdown points to the
// JSON report it summarizes.
func formats(fullReport string) *report.Registry {
	return report.NewRegistry(report.JSON{}, report.SARIF{}, report.Markdown{FullReport: fullReport})
}

// parseOutputs reads --output values of the form format=path.
func parseOutputs(values []string) ([]config.Output, error) {
	known := formats("").Names()
	var out []config.Output
	for _, v := range values {
		format, path, ok := strings.Cut(v, "=")
		switch {
		case !ok || format == "" || path == "":
			return nil, usageErrorf("--output %q is not of the form format=path", v)
		case !slices.Contains(known, format):
			return nil, usageErrorf("--output %q: unknown format %q (known: %s)", v, format, strings.Join(known, ", "))
		}
		out = append(out, config.Output{Format: format, Path: path})
	}
	return out, nil
}

// resolveOutputs is --output when given, else reporting.outputs, always
// including the canonical JSON report (at DefaultReportPath unless one is
// configured).
func resolveOutputs(flagOutputs, configured []config.Output) []config.Output {
	outputs := configured
	if len(flagOutputs) > 0 {
		outputs = flagOutputs
	}
	if !slices.ContainsFunc(outputs, func(o config.Output) bool { return o.Format == "json" }) {
		outputs = append(slices.Clone(outputs), config.Output{Format: "json", Path: DefaultReportPath})
	}
	return outputs
}

// writeReports writes every output. Failing to write a JSON report is
// fatal, as it is the canonical record; any other reporter failing is
// logged and never changes the outcome.
func (a app) writeReports(ctx context.Context, r *model.Report, outputs []config.Output) error {
	fullReport := DefaultReportPath
	for _, o := range outputs {
		if o.Format == "json" {
			fullReport = o.Path
			break
		}
	}
	registry := formats(fullReport)
	var fatal []error
	for _, o := range outputs {
		f, _ := registry.Lookup(o.Format) // formats were validated when the config loaded or --output was parsed
		data, err := f.Render(r)
		if err == nil {
			err = report.WriteFile(ctx, filepath.Join(a.inv.Dir, o.Path), data)
		}
		switch {
		case err == nil:
		case o.Format == "json":
			fatal = append(fatal, fmt.Errorf("write json report %s: %w", o.Path, err))
		default:
			a.logger.Warn("reporter failed", "format", o.Format, "path", o.Path, "error", err)
		}
	}
	return errors.Join(fatal...)
}
