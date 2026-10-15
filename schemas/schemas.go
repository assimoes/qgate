// Package schemas embeds qgate's JSON Schemas, the source of truth for its
// config, report and plugin wire formats.
package schemas

import _ "embed"

// Report is the JSON Schema for the canonical JSON report.
//
//go:embed report.schema.json
var Report []byte
