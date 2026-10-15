package cli

import (
	"bytes"
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
// decision decoded, the stored decision kept as raw bytes for an exact
// comparison.
type savedReport struct {
	Run            model.Run           `json:"run"`
	ResolvedConfig json.RawMessage     `json:"resolved_config"`
	Inputs         []model.InputStatus `json:"inputs"`
	Findings       []model.Finding     `json:"findings"`
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
		return usageErrorf("%s is not a qgate report: %v", path, err)
	}
	cfg, err := config.DecodeResolved(saved.ResolvedConfig)
	if err != nil {
		return usageErrorf("%s: %v", path, err)
	}
	var storedGate model.GateResult
	if err := json.Unmarshal(saved.Gate, &storedGate); err != nil {
		return usageErrorf("%s: invalid gate section: %v", path, err)
	}

	// Run health (input and agent failures, budgets, deadlines) is a fact
	// of the original run, recorded as gate.degraded.
	decision, err := pipeline.Decide(saved.Findings, cfg, saved.Inputs, storedGate.Degraded)
	if err != nil {
		return err
	}
	if err := sameJSON("scores", saved.Scores, decision.Scores); err != nil {
		return err
	}
	if err := sameJSON("gate", saved.Gate, decision.Gate); err != nil {
		return err
	}

	r := &model.Report{Run: saved.Run, Findings: saved.Findings, Scores: decision.Scores, Gate: decision.Gate}
	if err := (report.Console{W: a.inv.Stdout}).Report(cmd.Context(), r); err != nil {
		a.logger.Warn("console reporter failed", "error", err)
	}
	fmt.Fprintf(a.inv.Stdout, "Scores and gate reproduced exactly from %s\n", filepath.Base(path))
	return gateError(cfg.Profile.Enforcement, decision.Gate.Outcome)
}

// sameJSON compares a stored report section with a recomputed one, byte
// for byte after removing insignificant whitespace.
func sameJSON(section string, stored json.RawMessage, recomputed any) error {
	got, err := json.Marshal(recomputed)
	if err != nil {
		return fmt.Errorf("encode recomputed %s: %w", section, err)
	}
	var want bytes.Buffer
	if err := json.Compact(&want, stored); err != nil {
		return fmt.Errorf("stored %s section is not JSON: %w", section, err)
	}
	if !bytes.Equal(want.Bytes(), got) {
		return fmt.Errorf("recomputed %s do not match the report:\n  stored:     %s\n  recomputed: %s", section, want.Bytes(), got)
	}
	return nil
}
