package report

import (
	"encoding/json"

	"github.com/assimoes/qgate/internal/model"
)

// JSON is the canonical report (design §14.2), the input to `qgate score
// --report`.
type JSON struct{}

// Name is the format name outputs use.
func (JSON) Name() string { return "json" }

// Render encodes the whole report.
func (JSON) Render(r *model.Report) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
