package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"qgate/internal/model"
)

// JSON writes the canonical JSON report to a file.
type JSON struct {
	Path string
}

// Report writes r atomically: a reader never sees a half-written report.
func (j JSON) Report(_ context.Context, r *model.Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json report: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(j.Path), ".qgate-report-*.json")
	if err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write json report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	if err := os.Rename(tmp.Name(), j.Path); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}
	return nil
}
