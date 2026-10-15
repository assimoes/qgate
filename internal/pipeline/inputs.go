// Package pipeline runs the steps of `qgate run` (design §3.1) and
// assembles their results.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/ingest"
	"qgate/internal/model"
	"qgate/internal/normalize"
)

// maxParallelInputs bounds how many inputs are read and parsed at once.
const maxParallelInputs = 4

// Input statuses in the report.
const (
	InputOK     = "ok"
	InputFailed = "failed"
)

// InputDeps are what running inputs needs from the outside world.
type InputDeps struct {
	Registry   *ingest.Registry
	Dir        string   // repo root; input paths resolve against it
	Workspaces []string // absolute prefixes analyzers may put on paths
}

// InputsResult is every input's findings, metrics and status.
type InputsResult struct {
	Findings []model.Finding // per input in config order, each in canonical order
	Metrics  []model.Metric
	Statuses []model.InputStatus // in config order
}

// Failed reports whether any input failed.
func (r InputsResult) Failed() bool {
	for _, s := range r.Statuses {
		if s.Status == InputFailed {
			return true
		}
	}
	return false
}

// RunInputs reads and normalizes every configured input concurrently. A
// failing input is recorded in its status and never stops the others; only
// cancellation of ctx aborts the step.
func RunInputs(ctx context.Context, deps InputDeps, cfg config.Config, gc *gitctx.Context) (InputsResult, error) {
	type slot struct {
		findings []model.Finding
		metrics  []model.Metric
		status   model.InputStatus
	}
	slots := make([]slot, len(cfg.Inputs))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelInputs)
	for i, in := range cfg.Inputs {
		g.Go(func() error {
			findings, metrics, warnings, err := runInput(gctx, deps, cfg, gc, in)
			st := model.InputStatus{
				Name:     in.Name,
				Format:   in.Format,
				Status:   InputOK,
				Findings: len(findings),
				Metrics:  len(metrics),
				Warnings: append([]string{}, warnings...),
			}
			if err != nil {
				if ctxErr := gctx.Err(); ctxErr != nil {
					return ctxErr
				}
				msg := err.Error()
				st.Status, st.Error = InputFailed, &msg
				findings, metrics = nil, nil
			}
			slots[i] = slot{findings: findings, metrics: metrics, status: st}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return InputsResult{}, err
	}

	var res InputsResult
	for _, s := range slots {
		res.Findings = append(res.Findings, s.findings...)
		res.Metrics = append(res.Metrics, s.metrics...)
		res.Statuses = append(res.Statuses, s.status)
	}
	return res, nil
}

func runInput(ctx context.Context, deps InputDeps, cfg config.Config, gc *gitctx.Context, in config.Input) (
	[]model.Finding, []model.Metric, []string, error) {
	if in.Mode != "" && in.Mode != "import" {
		return nil, nil, nil, fmt.Errorf("input mode %q is not supported in this version", in.Mode)
	}
	ing, ok := deps.Registry.Lookup(in.Format)
	if !ok {
		return nil, nil, nil, fmt.Errorf("format %q is not supported in this version", in.Format)
	}

	p := in.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(deps.Dir, p)
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open %s: %w", in.Path, err)
	}
	defer f.Close()

	raw, err := ing.Parse(ctx, f, in, gc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parse %s: %w", in.Path, err)
	}
	findings, warnings, err := normalize.Tool(ctx, raw.Findings, normalize.Options{
		Input:       in,
		Project:     cfg.Project,
		GatingScope: cfg.Profile.GatingScope,
		Changes:     gc,
		Lines:       gc,
		Workspaces:  deps.Workspaces,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	for i := range findings {
		findings[i].Source.Name = in.Name
	}
	return findings, raw.Metrics, append(raw.Warnings, warnings...), nil
}
