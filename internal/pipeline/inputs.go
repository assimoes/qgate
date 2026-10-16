// Package pipeline runs the steps of `qgate run` (design §3.1) and
// assembles their results.
package pipeline

import (
	"context"
	"fmt"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/assimoes/qgate/internal/config"
	"github.com/assimoes/qgate/internal/gitctx"
	"github.com/assimoes/qgate/internal/ingest"
	"github.com/assimoes/qgate/internal/model"
	"github.com/assimoes/qgate/internal/normalize"
)

// maxParallelInputs bounds how many inputs are read and parsed at once.
const maxParallelInputs = 4

// InputDeps are what running inputs needs from the outside world.
type InputDeps struct {
	Registry   *ingest.Registry
	Dir        string            // repo root; input paths and commands resolve against it
	Env        map[string]string // qgate's environment; commands get a minimal subset
	Workspaces []string          // absolute prefixes analyzers may put on paths
}

// InputsResult is every input's findings, metrics and status.
type InputsResult struct {
	Findings []model.Finding // normalized up to gating scope, before excusal across inputs
	Metrics  []model.Metric
	Statuses []model.InputStatus // in config order
}

// RunInputs reads and normalizes every configured input concurrently. A
// failing input is recorded in its status and never stops the others; only
// cancellation of ctx aborts the step.
func RunInputs(ctx context.Context, deps InputDeps, cfg config.Config, gc *gitctx.Context) (InputsResult, error) {
	outcomes := make([]inputOutcome, len(cfg.Inputs))
	failures := make([]error, len(cfg.Inputs))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelInputs)
	for i, in := range cfg.Inputs {
		g.Go(func() error {
			out, err := runInput(gctx, deps, cfg, gc, in)
			if err != nil {
				if ctxErr := gctx.Err(); ctxErr != nil {
					return ctxErr
				}
				failures[i] = err
				return nil
			}
			outcomes[i] = out
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return InputsResult{}, err
	}

	res := InputsResult{Metrics: []model.Metric{}}
	var all []model.Finding
	for i, in := range cfg.Inputs {
		out := outcomes[i]
		kind := model.InputKindFindings
		if !deps.Registry.ProducesFindings(in.Format) {
			kind = model.InputKindMetrics
		}
		st := model.InputStatus{
			Name: in.Name, Format: in.Format, Kind: kind, Status: model.InputOK,
			Findings: len(out.findings), Metrics: len(out.metrics),
			Warnings: append([]string{}, out.warnings...),
		}
		if err := failures[i]; err != nil {
			msg := err.Error()
			st.Status, st.Error = model.InputFailed, &msg
		}
		all = append(all, out.findings...)
		res.Metrics = append(res.Metrics, out.metrics...)
		res.Statuses = append(res.Statuses, st)
	}
	res.Findings = all
	slices.SortStableFunc(res.Metrics, model.CompareMetrics)
	return res, nil
}

// inputOutcome is what one input produced.
type inputOutcome struct {
	findings []model.Finding
	metrics  []model.Metric
	warnings []string
}

func runInput(ctx context.Context, deps InputDeps, cfg config.Config, gc *gitctx.Context, in config.Input) (inputOutcome, error) {
	ing, ok := deps.Registry.Lookup(in.Format)
	if !ok {
		return inputOutcome{}, fmt.Errorf("format %q is not supported in this version", in.Format)
	}
	data, err := openInput(ctx, deps, in)
	if err != nil {
		return inputOutcome{}, err
	}
	defer data.Close()
	raw, err := ing.Parse(ctx, data, in, gc)
	if err != nil {
		if data.stderr != "" {
			return inputOutcome{}, fmt.Errorf("parse %s output: %w; stderr: %s", in.Format, err, data.stderr)
		}
		return inputOutcome{}, fmt.Errorf("parse %s output: %w", in.Format, err)
	}
	raw.Warnings = append(data.warnings, raw.Warnings...)
	findings, warnings, err := normalize.ToolFindings(ctx, raw.Findings, normalize.Options{
		Input:            in,
		CategoryMappings: cfg.CategoryMappings,
		Project:          cfg.Project,
		GatingScope:      cfg.Profile.GatingScope,
		Changes:          gc,
		Lines:            gc,
		Workspaces:       deps.Workspaces,
	})
	if err != nil {
		return inputOutcome{}, err
	}
	src := model.Source{Kind: model.SourceTool, Name: in.Name}
	for i := range findings {
		findings[i].Source.Name = in.Name
	}
	for i := range raw.Metrics {
		raw.Metrics[i].Source = src
	}
	return inputOutcome{findings: findings, metrics: raw.Metrics, warnings: append(raw.Warnings, warnings...)}, nil
}
