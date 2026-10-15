package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"qgate/internal/buildinfo"
	"qgate/internal/config"
	"qgate/internal/gitctx"
	"qgate/internal/model"
	"qgate/internal/pipeline"
	"qgate/internal/report"
)

// DefaultReportPath is where `qgate run` writes the JSON report when no
// output is configured, relative to the working directory.
const DefaultReportPath = "qgate-report.json"

// runFlags select what `qgate run` reviews.
type runFlags struct {
	base, head string
}

func (a app) newRunCommand() *cobra.Command {
	var flags configFlags
	var rf runFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full quality gate pipeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			started := time.Now()
			res, err := a.load(cmd, flags)
			if err != nil {
				return err
			}
			return a.runPipeline(cmd.Context(), started, res, rf)
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&rf.base, "base", "", "base commit or ref (default: merge base with origin's default branch)")
	cmd.Flags().StringVar(&rf.head, "head", "", "head commit or ref (default: HEAD)")
	return cmd
}

func (a app) runPipeline(ctx context.Context, started time.Time, res *config.Resolved, rf runFlags) error {
	if res.RepoConfig == "" {
		a.logger.Warn("no repo config found; only builtin policy is active")
	}
	cfg := res.Config

	// Git context is needed only when something produces findings.
	var gc *gitctx.Context
	inputs := pipeline.InputsResult{Findings: []model.Finding{}, Metrics: []model.Metric{}, Statuses: []model.InputStatus{}}
	if len(cfg.Inputs) > 0 {
		var err error
		gc, err = gitctx.Resolve(ctx, gitctx.Options{
			Dir: a.inv.Dir, Base: rf.base, Head: rf.head, Env: a.inv.Env,
			Include: cfg.Project.Include, Exclude: cfg.Project.Exclude,
		})
		if errors.Is(err, gitctx.ErrNotARepository) || errors.Is(err, gitctx.ErrNoBase) {
			return usageError{err}
		}
		if err != nil {
			return err
		}
		inputs, err = pipeline.RunInputs(ctx, pipeline.InputDeps{
			Registry:   a.inv.Ingestors,
			Dir:        a.inv.Dir,
			Workspaces: gc.Workspaces,
		}, cfg, gc)
		if err != nil {
			return err
		}
	}

	runID, err := newRunID()
	if err != nil {
		return err
	}
	// encoding/json sorts map keys, so this encoding is canonical.
	resolved, err := json.Marshal(res.Document)
	if err != nil {
		return fmt.Errorf("encode resolved config: %w", err)
	}
	hash := sha256.Sum256(resolved)
	decision, err := pipeline.Decide(inputs.Findings, cfg, inputs.Statuses, pipeline.Degraded(inputs.Statuses))
	if err != nil {
		return err
	}
	scoringParams, err := json.Marshal(res.Document["scoring"])
	if err != nil {
		return fmt.Errorf("encode scoring params: %w", err)
	}

	r := &model.Report{
		SchemaVersion: model.ReportSchemaVersion,
		QgateVersion:  buildinfo.Get().Version,
		Run: model.Run{
			ID:          runID,
			StartedAt:   started.UTC(),
			Stage:       cfg.Stage,
			Enforcement: cfg.Profile.Enforcement,
			GatingScope: cfg.Profile.GatingScope,
		},
		ConfigHash:     hex.EncodeToString(hash[:]),
		ResolvedConfig: resolved,
		Scoring:        model.Scoring{Version: cfg.Scoring.Version, Params: scoringParams},
		Inputs:         inputs.Statuses,
		Agents:         []model.AgentStatus{},
		Findings:       inputs.Findings,
		Metrics:        inputs.Metrics,
		Scores:         decision.Scores,
		Gate:           decision.Gate,
		DataEgress:     []model.Egress{},
	}
	if gc != nil {
		r.Run.BaseSHA, r.Run.HeadSHA = gc.BaseSHA, gc.HeadSHA
	}
	r.Run.DurationMS = time.Since(started).Milliseconds()

	// The JSON report is the canonical record: failing to write it is fatal.
	if err := (report.JSON{Path: filepath.Join(a.inv.Dir, DefaultReportPath)}).Report(ctx, r); err != nil {
		return err
	}
	// Any other reporter failing never changes the outcome.
	if err := (report.Console{W: a.inv.Stdout}).Report(ctx, r); err != nil {
		a.logger.Warn("console reporter failed", "error", err)
	}
	return gateError(r.Run.Enforcement, r.Gate.Outcome)
}

// gateError turns a failed gate into exit code 1. Only a fail outcome
// under blocking enforcement does; advisory runs never fail the job.
func gateError(e model.Enforcement, o model.Outcome) error {
	if e == model.EnforcementBlocking && o == model.OutcomeFail {
		return errGateFailed
	}
	return nil
}

// newRunID returns a random RFC 4122 version 4 UUID.
func newRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
