package model

import (
	"encoding/json"
	"time"
)

// ReportSchemaVersion is the version of the report wire format.
const ReportSchemaVersion = 1

// Enforcement says whether a profile's gate can block a merge.
type Enforcement string

// Enforcement levels.
const (
	EnforcementAdvisory Enforcement = "advisory"
	EnforcementBlocking Enforcement = "blocking"
)

// GatingScope says which findings can count as new.
type GatingScope string

// Gating scopes.
const (
	GatingScopeDiff  GatingScope = "diff"
	GatingScopeFiles GatingScope = "files"
	GatingScopeRepo  GatingScope = "repo"
)

// AgentKind is the role an agent plays.
type AgentKind string

// Agent kinds.
const (
	AgentKindReviewer AgentKind = "reviewer" // reads changed code and produces findings
	AgentKindTriager  AgentKind = "triager"  // annotates existing findings
)

// Outcome is the result of evaluating a gate.
type Outcome string

// Gate outcomes.
const (
	OutcomePass Outcome = "pass"
	OutcomeWarn Outcome = "warn"
	OutcomeFail Outcome = "fail"
)

// Report is the canonical record of one run (design §14.2).
type Report struct {
	SchemaVersion  int             `json:"schema_version"`
	QgateVersion   string          `json:"qgate_version"`
	Run            Run             `json:"run"`
	ConfigHash     string          `json:"config_hash"`
	ResolvedConfig json.RawMessage `json:"resolved_config"`
	Scoring        Scoring         `json:"scoring"`
	Inputs         []InputStatus   `json:"inputs"`
	Agents         []AgentStatus   `json:"agents"`
	Findings       []Finding       `json:"findings"`
	Metrics        []Metric        `json:"metrics"`
	Scores         Scores          `json:"scores"`
	Gate           GateResult      `json:"gate"`
	DataEgress     []Egress        `json:"data_egress"`
}

// Run is the report's run metadata.
type Run struct {
	ID          string      `json:"id"`
	StartedAt   time.Time   `json:"started_at"`
	DurationMS  int64       `json:"duration_ms"`
	Stage       string      `json:"stage"`
	Enforcement Enforcement `json:"enforcement"`
	GatingScope GatingScope `json:"scope"`
	BaseSHA     string      `json:"base_sha,omitempty"`
	HeadSHA     string      `json:"head_sha,omitempty"`
	PR          int         `json:"pr,omitempty"`
}

// Scoring records the algorithm version and parameters used.
type Scoring struct {
	Version int             `json:"version"`
	Params  json.RawMessage `json:"params"`
}

// InputState says whether an input produced its results.
type InputState string

// Input states.
const (
	InputOK     InputState = "ok"
	InputFailed InputState = "failed"
)

// InputStatus summarizes one input's run.
type InputStatus struct {
	Name     string     `json:"name"`
	Format   string     `json:"format"`
	Status   InputState `json:"status"`
	Findings int        `json:"findings"`
	Metrics  int        `json:"metrics"`
	Warnings []string   `json:"warnings"`
	Error    *string    `json:"error"`
}

// AgentStatus summarizes one agent's run.
type AgentStatus struct {
	Name   string    `json:"name"`
	Kind   AgentKind `json:"kind"`
	Status string    `json:"status"`
}

// Scores are reported as whole points; nil means not assessed.
type Scores struct {
	Overall    *int            `json:"overall"`
	Categories map[string]*int `json:"categories"`
}

// GateResult is the gate outcome and every gate check evaluated.
type GateResult struct {
	Outcome  Outcome     `json:"outcome"`
	Degraded bool        `json:"degraded"`
	Checks   []GateCheck `json:"checks"`
}

// CheckResult is how one gate check came out.
type CheckResult string

// Gate check results.
const (
	CheckPass    CheckResult = "pass"
	CheckFail    CheckResult = "fail"
	CheckSkipped CheckResult = "skipped" // could not apply, e.g. category not assessed
)

// GateCheck is the result of one gate check.
type GateCheck struct {
	Check    string      `json:"check"`
	Expected string      `json:"expected"`
	Actual   any         `json:"actual"` // a number, count or boolean; null when unknown
	Result   CheckResult `json:"result"`
	Note     string      `json:"note,omitempty"`
}

// Egress lists the files sent to one provider.
type Egress struct {
	Provider string   `json:"provider"`
	Files    []string `json:"files"`
}
