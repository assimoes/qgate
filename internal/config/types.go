package config

import (
	"encoding/json"
	"fmt"
	"time"

	"qgate/internal/model"
)

// Config is the resolved configuration for one run: every layer merged, the
// stage's profile resolved and --set applied. Its shape mirrors
// $defs/resolved in the config schema.
type Config struct {
	Version          int                 `json:"version"`
	Stage            string              `json:"stage"`
	Profile          Profile             `json:"profile"`
	Project          Project             `json:"project"`
	Scoring          Scoring             `json:"scoring"`
	LLM              LLM                 `json:"llm"`
	Providers        map[string]Provider `json:"providers"`
	Inputs           []Input             `json:"inputs"`
	CategoryMappings []CategoryMapping   `json:"category_mappings"`
	Agents           map[string]Agent    `json:"agents"`
	ReviewRules      []ReviewRule        `json:"review_rules"`
	Suppressions     []Suppression       `json:"suppressions"`
	Baseline         Baseline            `json:"baseline"`
	Reporting        Reporting           `json:"reporting"`
}

// Profile is the resolved profile the stage selected.
type Profile struct {
	Enforcement          model.Enforcement `json:"enforcement"`
	GatingScope          model.GatingScope `json:"scope"`
	Agents               []string          `json:"agents"`
	OnInputError         ErrorPolicy       `json:"on_input_error"`
	OnAgentError         ErrorPolicy       `json:"on_agent_error"`
	AgentContextFindings bool              `json:"agent_context_findings"`
	Gate                 Gate              `json:"gate"`
}

// ErrorPolicy says whether an input or agent failure is fatal.
type ErrorPolicy string

// Error policies.
const (
	ErrorPolicyWarn ErrorPolicy = "warn"
	ErrorPolicyFail ErrorPolicy = "fail"
)

// Gate is a profile's gate checks.
type Gate struct {
	OverallMin      *int                   `json:"overall_min"`
	CategoryMin     map[model.Category]int `json:"category_min"`
	MaxNew          map[model.Severity]int `json:"max_new"`
	MaxNewTool      map[model.Severity]int `json:"max_new_tool"`
	RequireAssessed []model.Category       `json:"require_assessed"`
	Metrics         []MetricCheck          `json:"metrics"`
}

// MetricCheck is a gate check on a metric.
type MetricCheck struct {
	Name     string            `json:"name"`
	Scope    model.MetricScope `json:"scope"`
	Op       string            `json:"op"`
	Value    float64           `json:"value"`
	Optional bool              `json:"optional"`
}

// Project selects which files qgate considers.
type Project struct {
	Name    string   `json:"name"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// Scoring holds the scoring algorithm version and its parameters.
type Scoring struct {
	Version            int                    `json:"version"`
	SeverityWeights    map[model.Severity]int `json:"severity_weights"`
	AgentWeightPct     int                    `json:"agent_weight_pct"`
	AgentMinConfidence int                    `json:"agent_min_confidence"`
	AgentMaxPenalty    int                    `json:"agent_max_penalty"`
	CategoryWeights    map[model.Category]int `json:"category_weights"`
}

// LLM holds settings that apply to every provider.
type LLM struct {
	AllowedProviders []string `json:"allowed_providers"`
	RedactSecrets    bool     `json:"redact_secrets"`
	RedactPatterns   []string `json:"redact_patterns"`
	ExcludePaths     []string `json:"exclude_paths"`
	Budget           Budget   `json:"budget"`
}

// Budget bounds LLM usage per run.
type Budget struct {
	MaxInputTokensPerRun int `json:"max_input_tokens_per_run"`
	MaxUnitsPerAgent     int `json:"max_units_per_agent"`
}

// Provider configures one LLM backend. API keys are only ever referenced by
// environment variable name.
type Provider struct {
	Type           string       `json:"type"`
	APIKeyEnv      string       `json:"api_key_env"`
	BaseURL        string       `json:"base_url"`
	APIVersion     string       `json:"api_version"`
	MaxConcurrency int          `json:"max_concurrency"`
	RequestTimeout Duration     `json:"request_timeout"`
	Fallback       string       `json:"fallback"`
	Capabilities   Capabilities `json:"capabilities"`
	Pricing        *Pricing     `json:"pricing"`
}

// Capabilities are what a provider's model is declared to support.
type Capabilities struct {
	ContextWindow int  `json:"context_window"`
	JSONSchema    bool `json:"json_schema"`
	ToolCalls     bool `json:"tool_calls"`
}

// Pricing is an optional price table for cost reporting.
type Pricing struct {
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
}

// Input brings an analyzer's results into qgate.
type Input struct {
	Name            string                    `json:"name"`
	Mode            string                    `json:"mode"`
	Format          string                    `json:"format"`
	Path            string                    `json:"path"`
	Command         []string                  `json:"command"`
	Timeout         Duration                  `json:"timeout"`
	Env             []string                  `json:"env"`
	Required        bool                      `json:"required"`
	CategoryDefault model.Category            `json:"category_default"`
	Assesses        []model.Category          `json:"assesses"`
	SeverityMap     map[string]model.Severity `json:"severity_map"`
	Mapping         *JSONMapping              `json:"mapping"`
	Options         map[string]any            `json:"options"`
}

// EffectiveCategory is where the input's findings go and what it assesses
// when no category mapping applies: its category_default, else the default
// category.
func (in Input) EffectiveCategory() model.Category {
	if in.CategoryDefault != "" {
		return in.CategoryDefault
	}
	return model.DefaultCategory
}

// JSONMapping maps arbitrary JSON to findings with JSONPath expressions.
type JSONMapping struct {
	Items     string `json:"items"`
	RuleID    string `json:"rule_id"`
	Message   string `json:"message"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	StartLine string `json:"start_line"`
	EndLine   string `json:"end_line"`
	Severity  string `json:"severity"`
}

// CategoryMapping assigns a category to findings matching a pattern.
type CategoryMapping struct {
	Match    CategoryMatch  `json:"match"`
	Category model.Category `json:"category"`
}

// CategoryMatch is what a category mapping matches on.
type CategoryMatch struct {
	CWE    string `json:"cwe"`
	RuleID string `json:"rule_id"`
	Path   string `json:"path"`
}

// Agent configures one reviewer or triager.
type Agent struct {
	Kind               model.AgentKind  `json:"kind"`
	Provider           string           `json:"provider"`
	Model              string           `json:"model"`
	Prompt             Prompt           `json:"prompt"`
	Categories         []model.Category `json:"categories"`
	MaxFindingsPerUnit int              `json:"max_findings_per_unit"`
	MaxSeverity        *model.Severity  `json:"max_severity"`
	PriorityPaths      []string         `json:"priority_paths"`
	AppliesTo          AppliesTo        `json:"applies_to"`
	IncludeFullFile    bool             `json:"include_full_file"`
	ContextLines       *int             `json:"context_lines"`
}

// Prompt is an agent's role prompt and its extensions.
type Prompt struct {
	Base   string   `json:"base"`
	Extend []string `json:"extend"`
}

// AppliesTo selects the findings a triager reviews.
type AppliesTo struct {
	MinSeverity *model.Severity `json:"min_severity"`
}

// ReviewRule is a natural-language requirement given to matching reviewers.
type ReviewRule struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Severity    model.Severity `json:"severity"`
	Category    model.Category `json:"category"`
	Paths       []string       `json:"paths"`
	Agents      []string       `json:"agents"`
}

// Suppression excuses findings matching a rule and path, with a reason.
type Suppression struct {
	Rule    string   `json:"rule"`
	Paths   []string `json:"paths"`
	Reason  string   `json:"reason"`
	Expires string   `json:"expires"` // YYYY-MM-DD
}

// Baseline points at a baseline file.
type Baseline struct {
	Path string `json:"path"`
}

// Reporting configures PR comments and output files.
type Reporting struct {
	PR      PRReporting `json:"pr"`
	Outputs []Output    `json:"outputs"`
}

// PRReporting configures PR summary and inline comments.
type PRReporting struct {
	Summary           bool            `json:"summary"`
	Inline            bool            `json:"inline"`
	InlineMinSeverity *model.Severity `json:"inline_min_severity"`
	MaxInlineComments int             `json:"max_inline_comments"`
	Platform          string          `json:"platform"`
	APIURL            string          `json:"api_url"`
}

// Output is one configured report file.
type Output struct {
	Format string `json:"format"`
	Path   string `json:"path"`
}

// Duration is a time.Duration written as a Go duration string ("90s").
type Duration time.Duration

// UnmarshalJSON parses a Go duration string.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(v)
	return nil
}
