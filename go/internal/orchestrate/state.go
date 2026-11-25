package orchestrate

import "time"

// Phase is a deterministic orchestrate state-machine step.
type Phase string

const (
	PhaseClassify  Phase = "classify"
	PhaseAnswer    Phase = "answer"
	PhaseRoute     Phase = "route"
	PhaseWorker    Phase = "worker"
	PhaseRetry     Phase = "retry"
	PhasePresent   Phase = "present"
	PhaseAwaitUser Phase = "await_user"
	PhaseDone      Phase = "done"
	PhaseFailed    Phase = "failed"
)

const (
	StatusCompleted   = "completed"
	StatusAwaitingUser = "awaiting_user"
	StatusPartial     = "partial"
	StatusFailed      = "failed"
)

// AskInput is the public sc_ask request.
type AskInput struct {
	Question    string
	Primary     string
	Slots       map[string]any
	DetailLevel string
	TraceLevel  string // none | compact | full
}

// BrowserGate tells the host whether/when to open interactive sites.
type BrowserGate struct {
	Needed        bool             `json:"needed"`
	WaitConfirm   bool             `json:"wait_confirm"`
	LeaveOpen     bool             `json:"leave_open"`
	HandoffToUser bool             `json:"handoff_to_user"`
	Site          string           `json:"site,omitempty"`
	StartURL      string           `json:"start_url,omitempty"`
	SearchTerm    string           `json:"search_term,omitempty"`
	Resources     []map[string]any `json:"resources,omitempty"`
}

// FactBlock is one verifiable fact for answer_blocks.
type FactBlock struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// AnswerBlocks is a structured draft scaffold for hosts.
type AnswerBlocks struct {
	Headline string      `json:"headline"`
	Facts    []FactBlock `json:"facts"`
	Warnings []string    `json:"warnings,omitempty"`
	Sources  []string    `json:"sources,omitempty"`
	Version  string      `json:"version,omitempty"`
	Missing  []string    `json:"missing,omitempty"`
}

// TraceEvent is one full-trace decision record.
type TraceEvent struct {
	Phase      Phase  `json:"phase"`
	Decision   string `json:"decision,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// TraceSummary is the compact (or full) execution trace.
type TraceSummary struct {
	Phases         []Phase      `json:"phases"`
	Mode           string       `json:"mode"`
	ReplanCount    int          `json:"replan_count"`
	SlotRetryCount int          `json:"slot_retry_count"`
	DurationMs     int64        `json:"duration_ms"`
	Events         []TraceEvent `json:"events,omitempty"`
}

// AskResult is the flat sc_ask response envelope.
type AskResult struct {
	OK           bool             `json:"ok"`
	Status       string           `json:"status"`
	Primary      string           `json:"primary"`
	Slots        map[string]any   `json:"slots"`
	Path         string           `json:"path"`
	AnswerDraft  string           `json:"answer_draft"`
	AnswerBlocks AnswerBlocks     `json:"answer_blocks"`
	Evidence     []map[string]any `json:"evidence,omitempty"`
	Meta         map[string]any   `json:"meta"`
	Browser      BrowserGate      `json:"browser"`
	FollowUps    []string         `json:"follow_ups,omitempty"`
	Trace        TraceSummary     `json:"trace"`
	Error        *AskError        `json:"error,omitempty"`
}

// Policy holds hard limits for the state machine.
type Policy struct {
	MaxSteps       int
	MaxReplan      int
	MaxSlotRetry   int
	AllowRetry     bool
}

// DefaultPolicy returns v1 limits from the design doc.
func DefaultPolicy() Policy {
	return Policy{
		MaxSteps:     8,
		MaxReplan:    1,
		MaxSlotRetry: 1,
		AllowRetry:   true,
	}
}

// State is the in-memory orchestration state for one Ask.
type State struct {
	Question string
	Primary  string
	Slots    map[string]any
	Detail   string
	Phase    Phase

	PlanIntent string
	Bypass     bool
	Mode       string // answer_bypass | route_worker

	Answer   map[string]any
	Plan     map[string]any
	Worker   map[string]any
	Evidence []map[string]any
	Meta     map[string]any
	Browser  BrowserGate

	Draft      string
	Blocks     AnswerBlocks
	FollowUps  []string
	Status     string
	Path       string
	Warnings   []string
	Missing    []string
	FailReason string
	AskError   *AskError

	ReplanCount    int
	SlotRetryCount int
	RetryKind      string // replan | slot_retry | ""
	Trace          []TraceEvent

	startedAt time.Time
}

func terminal(p Phase) bool {
	return p == PhaseDone || p == PhaseFailed || p == PhaseAwaitUser
}
