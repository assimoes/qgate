package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"qgate/internal/config"
	"qgate/internal/model"
	"qgate/internal/pipeline"
	"qgate/internal/report"
)

// savedReport is a report as `score --report` reads it: the inputs to the
// decision decoded, the stored decision and config kept raw for exact
// comparison.
type savedReport struct {
	Run            model.Run           `json:"run"`
	ConfigHash     string              `json:"config_hash"`
	ResolvedConfig json.RawMessage     `json:"resolved_config"`
	Scoring        json.RawMessage     `json:"scoring"`
	Inputs         []model.InputStatus `json:"inputs"`
	Findings       []model.Finding     `json:"findings"`
	Metrics        []model.Metric      `json:"metrics"`
	Scores         json.RawMessage     `json:"scores"`
	Gate           json.RawMessage     `json:"gate"`
}

func (a app) newScoreCommand() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "score --report <file>",
		Short: "Recompute scores and the gate from a saved report, without analyzers or LLMs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.score(cmd, path)
		},
	}
	cmd.Flags().StringVar(&path, "report", "", "saved JSON report (required)")
	_ = cmd.MarkFlagRequired("report")
	return cmd
}

func (a app) score(cmd *cobra.Command, path string) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.inv.Dir, path)
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return usageErrorf("report %s not found", path)
	case err != nil:
		return fmt.Errorf("read report: %w", err)
	}
	var saved savedReport
	if err := json.Unmarshal(data, &saved); err != nil {
		return usageErrorf("%s is not a qgate report: %w", path, err)
	}
	cfg, err := config.DecodeResolved(saved.ResolvedConfig)
	if err != nil {
		return usageErrorf("%s: %w", path, err)
	}
	var storedGate model.GateResult
	if err := json.Unmarshal(saved.Gate, &storedGate); err != nil {
		return usageErrorf("%s: invalid gate section: %w", path, err)
	}
	if err := saved.checkConsistency(storedGate); err != nil {
		return err
	}

	// Run health beyond input statuses (agent failures, budgets,
	// deadlines) is a fact of the original run, recorded as gate.degraded.
	decision, err := pipeline.Decide(pipeline.Evidence{
		Findings: saved.Findings, Metrics: saved.Metrics, Inputs: saved.Inputs, Degraded: storedGate.Degraded,
	}, cfg)
	if err != nil {
		return fmt.Errorf("recompute decision: %w", err)
	}
	if err := sameJSON("scores", saved.Scores, decision.Scores); err != nil {
		return err
	}
	if err := sameJSON("gate", saved.Gate, decision.Gate); err != nil {
		return err
	}

	r := &model.Report{Run: saved.Run, Findings: saved.Findings, Scores: decision.Scores, Gate: decision.Gate}
	console := report.Console{W: a.inv.Stdout, ReproducedFrom: filepath.Base(path)}
	if err := console.Report(cmd.Context(), r); err != nil {
		a.logger.Warn("console reporter failed", "error", err)
	}
	return gateError(cfg.Profile.Enforcement, decision.Gate.Outcome)
}

// checkConsistency verifies the report against itself before trusting the
// config it carries: the config hash, the scoring section and the degraded
// flag. A report is not signed, so this catches edits that leave it
// inconsistent, not a forger who rewrites every section to agree.
func (s savedReport) checkConsistency(storedGate model.GateResult) error {
	config, err := canonicalJSON(s.ResolvedConfig)
	if err != nil {
		return fmt.Errorf("resolved_config is not JSON: %w", err)
	}
	sum := sha256.Sum256(config)
	if got := hex.EncodeToString(sum[:]); got != s.ConfigHash {
		return fmt.Errorf("resolved_config does not match config_hash: the config in the report was changed")
	}

	var resolved struct {
		Scoring json.RawMessage `json:"scoring"`
	}
	if err := json.Unmarshal(s.ResolvedConfig, &resolved); err != nil {
		return fmt.Errorf("resolved_config: %w", err)
	}
	var scoring struct {
		Version json.Number     `json:"version"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(s.Scoring, &scoring); err != nil {
		return fmt.Errorf("scoring section: %w", err)
	}
	storedParams, err := canonicalJSON(scoring.Params)
	if err != nil {
		return fmt.Errorf("scoring section: %w", err)
	}
	want, err := canonicalJSON(resolved.Scoring)
	if err != nil {
		return fmt.Errorf("resolved_config.scoring: %w", err)
	}
	var cfgScoring struct {
		Version json.Number `json:"version"`
	}
	_ = json.Unmarshal(resolved.Scoring, &cfgScoring)
	if !bytes.Equal(storedParams, want) || scoring.Version != cfgScoring.Version {
		return fmt.Errorf("the scoring section does not match resolved_config.scoring")
	}

	if pipeline.Degraded(s.Inputs) && !storedGate.Degraded {
		return fmt.Errorf("gate.degraded is false although an input failed")
	}
	return nil
}

// sameJSON compares a stored report section with a recomputed one, byte for
// byte in canonical form (sorted keys, no insignificant whitespace), so a
// report re-encoded by another tool still compares equal.
func sameJSON(section string, stored json.RawMessage, recomputed any) error {
	want, err := canonicalJSON(stored)
	if err != nil {
		return fmt.Errorf("stored %s section is not JSON: %w", section, err)
	}
	raw, err := json.Marshal(recomputed)
	if err != nil {
		return fmt.Errorf("encode recomputed %s: %w", section, err)
	}
	got, err := canonicalJSON(raw)
	if err != nil {
		return fmt.Errorf("encode recomputed %s: %w", section, err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("recomputed %s do not match the report:\n  stored:     %s\n  recomputed: %s", section, want, got)
	}
	return nil
}

// canonicalJSON re-encodes JSON with sorted keys and no whitespace, keeping
// numbers exactly as written. It is the same encoding config_hash uses.
func canonicalJSON(data []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
