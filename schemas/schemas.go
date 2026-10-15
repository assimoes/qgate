// Package schemas embeds qgate's JSON Schemas, the source of truth for its
// config, report and plugin wire formats.
package schemas

import _ "embed"

// Report is the JSON Schema for the canonical JSON report.
//
//go:embed report.schema.json
var Report []byte

// Config is the JSON Schema for config layers ($defs/layer, the root) and
// the resolved config ($defs/resolved).
//
//go:embed config.schema.json
var Config []byte
