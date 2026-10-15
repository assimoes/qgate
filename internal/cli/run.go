package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"qgate/internal/buildinfo"
	"qgate/internal/config"
	"qgate/internal/model"
	"qgate/internal/report"
)

// DefaultReportPath is where `qgate run` writes the JSON report when no
// output is configured, relative to the working directory.
const DefaultReportPath = "qgate-report.json"

func (a app) newRunCommand() *cobra.Command {
	var flags configFlags
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
			return a.runPipeline(cmd.Context(), started, res)
		},
	}
	flags.register(cmd)
	return cmd
}

func (a app) runPipeline(ctx context.Context, started time.Time, res *config.Resolved) error {
	if res.RepoConfig == "" {
		a.logger.Warn("no repo config found; only builtin policy is active")
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
	cfg := res.Config
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
		Inputs:         []model.InputStatus{},
		Agents:         []model.AgentStatus{},
		Findings:       []model.Finding{},
		Metrics:        []model.Metric{},
		Scores:         model.Scores{Categories: map[string]*int{}},
		Gate:           model.GateResult{Outcome: model.OutcomePass, Checks: []model.GateCheck{}},
		DataEgress:     []model.Egress{},
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
