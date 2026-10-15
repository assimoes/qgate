package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"qgate/internal/buildinfo"
	"qgate/internal/model"
	"qgate/internal/report"
	"qgate/internal/score"
)

// DefaultReportPath is where `qgate run` writes the JSON report when no
// output is configured, relative to the working directory.
const DefaultReportPath = "qgate-report.json"

const defaultStage = "dev"

// repoConfigNames are the repo config files looked up in the repo root, in order.
var repoConfigNames = []string{".qgate.yaml", ".qgate.yml"}

type runOptions struct {
	stage string
}

func (a app) newRunCommand() *cobra.Command {
	var opts runOptions
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full quality gate pipeline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("stage") {
				opts.stage = envOr(a.inv.Env, "QGATE_STAGE", defaultStage)
			}
			return a.runPipeline(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.stage, "stage", defaultStage, "stage to run; selects the profile of the same name (env QGATE_STAGE)")
	return cmd
}

func (a app) runPipeline(ctx context.Context, opts runOptions) error {
	started := time.Now()

	found, err := a.repoConfigExists()
	if err != nil {
		return err
	}
	if !found {
		a.logger.Warn("no repo config found; only builtin policy is active")
	}

	runID, err := newRunID()
	if err != nil {
		return err
	}
	resolved := json.RawMessage(`{}`)
	hash := sha256.Sum256(resolved)

	r := &model.Report{
		SchemaVersion: model.ReportSchemaVersion,
		QgateVersion:  buildinfo.Get().Version,
		Run: model.Run{
			ID:          runID,
			StartedAt:   started.UTC(),
			Stage:       opts.stage,
			Enforcement: model.EnforcementAdvisory,
			GatingScope: model.GatingScopeDiff,
		},
		ConfigHash:     hex.EncodeToString(hash[:]),
		ResolvedConfig: resolved,
		Scoring:        model.Scoring{Version: score.AlgorithmVersion, Params: json.RawMessage(`{}`)},
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

// repoConfigExists reports whether a repo config is present. An explicitly
// named config (QGATE_CONFIG) that does not exist is a config error.
func (a app) repoConfigExists() (bool, error) {
	if p := a.inv.Env["QGATE_CONFIG"]; p != "" {
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.inv.Dir, p)
		}
		if _, err := os.Stat(p); err != nil {
			return false, usageErrorf("repo config %s: %w", p, err)
		}
		return true, nil
	}
	for _, name := range repoConfigNames {
		_, err := os.Stat(filepath.Join(a.inv.Dir, name))
		switch {
		case err == nil:
			return true, nil
		case !errors.Is(err, fs.ErrNotExist):
			return false, fmt.Errorf("look up repo config: %w", err)
		}
	}
	return false, nil
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

func envOr(env map[string]string, key, fallback string) string {
	if v := env[key]; v != "" {
		return v
	}
	return fallback
}
