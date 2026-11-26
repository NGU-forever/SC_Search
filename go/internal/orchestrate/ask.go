package orchestrate

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/frankxiong/sc-search/internal/anyx"
	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/worker"
)

// Engine runs the Ask state machine against local DB tools.
type Engine struct {
	DB      *sql.DB
	Invoker worker.Invoker
	Policy  Policy
}

// NewEngine builds an Engine with native worker invoker and default policy.
func NewEngine(db *sql.DB) *Engine {
	return &Engine{
		DB:      db,
		Invoker: worker.NewNativeInvoker(db),
		Policy:  DefaultPolicy(),
	}
}

// Ask executes the one-shot orchestration pipeline.
func (e *Engine) Ask(ctx context.Context, in AskInput) AskResult {
	if e == nil || e.DB == nil {
		return AskResult{
			OK: false, Status: StatusFailed,
			Slots: map[string]any{}, Meta: map[string]any{},
			Error: askErr(ErrDatabaseMissing, "database unavailable"),
		}
	}
	q := strings.TrimSpace(in.Question)
	if q == "" {
		return AskResult{
			OK: false, Status: StatusFailed,
			Slots: map[string]any{}, Meta: map[string]any{},
			Error: askErr(ErrInvalidInput, "question is required"),
		}
	}
	detail := strings.ToLower(strings.TrimSpace(in.DetailLevel))
	if detail != "brief" && detail != "full" {
		detail = "brief"
	}
	traceLevel := strings.ToLower(strings.TrimSpace(in.TraceLevel))
	if traceLevel == "" {
		traceLevel = "compact"
	}

	st := &State{
		Question:  q,
		Primary:   strings.TrimSpace(in.Primary),
		Slots:     map[string]any{},
		Detail:    detail,
		Phase:     PhaseClassify,
		Meta:      map[string]any{},
		startedAt: time.Now(),
	}
	if in.Slots != nil {
		for k, v := range in.Slots {
			st.Slots[k] = v
		}
	}
	if e.Invoker == nil {
		e.Invoker = worker.NewNativeInvoker(e.DB)
	}
	pol := e.Policy
	if pol.MaxSteps <= 0 {
		pol = DefaultPolicy()
	}

	steps := 0
	for !terminal(st.Phase) {
		if steps >= pol.MaxSteps {
			st.AskError = askErr(ErrStepLimit, "orchestration step limit exceeded")
			st.Status = StatusFailed
			st.Phase = PhaseFailed
			break
		}
		steps++
		start := time.Now()
		from := st.Phase
		next, decision, reason, err := e.runPhase(ctx, st)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			st.AskError = askErr(ErrInternal, err.Error())
			st.FailReason = err.Error()
			st.Status = StatusFailed
			next = PhaseFailed
			decision = "error"
			reason = err.Error()
		}
		st.Trace = append(st.Trace, TraceEvent{
			Phase: from, Decision: decision, Reason: reason, DurationMs: dur,
		})
		st.Phase = next
	}

	return e.finish(st, traceLevel)
}

func (e *Engine) runPhase(ctx context.Context, st *State) (Phase, string, string, error) {
	switch st.Phase {
	case PhaseClassify:
		next, decision, err := classify(e.DB, st)
		return next, decision, st.Primary, err
	case PhaseAnswer:
		ans, err := query.AnswerQuestion(e.DB, st.Question, st.Detail, "")
		if err != nil {
			return PhaseFailed, "answer_error", err.Error(), err
		}
		st.Answer = ans
		if path := asString(ans["path"]); path != "" {
			st.Path = path
		}
		if anyx.Bool(ans["route_required"]) {
			if p := asString(ans["suggested_primary"]); p != "" {
				st.Primary = p
			}
			st.Mode = "route_worker"
			st.Bypass = false
			return PhaseRoute, "route_required", st.Primary, nil
		}
		if ev, ok := ans["evidence"].(map[string]any); ok {
			st.Evidence = evidenceFromAnswer(ev)
		}
		st.Meta = metaFromAnswer(ans)
		return PhasePresent, "answer_ok", asString(ans["intent"]), nil
	case PhaseRoute:
		plan, err := routePlanToMap(st.Question, st.Primary, st.Slots, st.ReplanCount)
		if err != nil {
			st.AskError = askErr(ErrRouteFailed, err.Error())
			return PhaseFailed, "route_error", err.Error(), err
		}
		st.Plan = plan
		st.Primary = asString(plan["primary"])
		if slots, ok := plan["slots"].(map[string]any); ok {
			st.Slots = slots
		}
		st.Path = asString(plan["path"])
		return PhaseWorker, "route_ok", st.Primary, nil
	case PhaseWorker:
		wr := worker.RunPlan(ctx, st.Plan, e.Invoker)
		st.Worker = wr
		st.Evidence = anyx.Maps(wr["evidence"])
		if meta, ok := wr["meta"].(map[string]any); ok {
			st.Meta = meta
		}
		if path := asString(wr["path"]); path != "" {
			st.Path = path
		}
		if anyx.Bool(wr["replan_suggested"]) || anyx.Bool(wr["slot_retry_suggested"]) {
			return PhaseRetry, "retry_signal", retryReason(wr), nil
		}
		return PhasePresent, "worker_ok", st.Primary, nil
	case PhaseRetry:
		if !e.Policy.AllowRetry {
			st.Warnings = append(st.Warnings, "retry disabled")
			return PhasePresent, "retry_disabled", "", nil
		}
		if !applyRetry(st) {
			return PhasePresent, "retry_exhausted", st.RetryKind, nil
		}
		return PhaseRoute, st.RetryKind, st.Primary, nil
	case PhasePresent:
		next := present(st)
		return next, "present", st.Status, nil
	default:
		st.AskError = askErr(ErrInternal, "unknown phase "+string(st.Phase))
		return PhaseFailed, "unknown_phase", string(st.Phase), nil
	}
}

func (e *Engine) finish(st *State, traceLevel string) AskResult {
	phases := make([]Phase, 0, len(st.Trace))
	for _, ev := range st.Trace {
		phases = append(phases, ev.Phase)
	}
	if st.Mode == "" {
		if st.Bypass {
			st.Mode = "answer_bypass"
		} else {
			st.Mode = "route_worker"
		}
	}
	if st.Slots == nil {
		st.Slots = map[string]any{}
	}
	if st.Meta == nil {
		st.Meta = map[string]any{}
	}
	if st.Status == "" {
		if st.AskError != nil {
			st.Status = StatusFailed
		} else if st.Phase == PhaseAwaitUser {
			st.Status = StatusAwaitingUser
		} else {
			st.Status = StatusCompleted
		}
	}

	trace := TraceSummary{
		Phases:         phases,
		Mode:           st.Mode,
		ReplanCount:    st.ReplanCount,
		SlotRetryCount: st.SlotRetryCount,
		DurationMs:     time.Since(st.startedAt).Milliseconds(),
	}
	if traceLevel == "full" {
		trace.Events = st.Trace
	} else if traceLevel == "none" {
		trace = TraceSummary{}
	}

	ok := st.AskError == nil || st.AskError.Code == ErrNoMatch
	if st.Status == StatusFailed && st.AskError != nil && st.AskError.Code != ErrNoMatch {
		ok = false
	} else if st.Status != StatusFailed {
		ok = true
	}

	return AskResult{
		OK:           ok,
		Status:       st.Status,
		Primary:      st.Primary,
		Slots:        st.Slots,
		Path:         st.Path,
		AnswerDraft:  st.Draft,
		AnswerBlocks: st.Blocks,
		Evidence:     st.Evidence,
		Meta:         st.Meta,
		Browser:      st.Browser,
		FollowUps:    st.FollowUps,
		Trace:        trace,
		Error:        st.AskError,
	}
}

func evidenceFromAnswer(ev map[string]any) []map[string]any {
	out := []map[string]any{}
	if facts := anyx.Maps(ev["facts"]); len(facts) > 0 {
		out = append(out, map[string]any{"tool": "sc_entity_facts", "ok": true, "data": map[string]any{"facts": facts}})
	}
	if prices := anyx.Maps(ev["prices"]); len(prices) > 0 {
		out = append(out, map[string]any{"tool": "sc_best_prices", "ok": true, "data": prices})
	}
	if ents := anyx.Maps(ev["entities"]); len(ents) > 0 {
		out = append(out, map[string]any{"tool": "sc_resolve_name", "ok": true, "data": ents})
	}
	return out
}

func metaFromAnswer(ans map[string]any) map[string]any {
	meta := map[string]any{}
	if v := asString(ans["live_game_version"]); v != "" {
		meta["as_of"] = v
	}
	if plan, ok := ans["plan"].(map[string]any); ok {
		if b, ok := plan["browser"].(map[string]any); ok {
			_ = b
		}
	}
	if ev, ok := ans["evidence"].(map[string]any); ok {
		if m, ok := ev["meta"].(map[string]any); ok {
			for k, v := range m {
				meta[k] = v
			}
		}
	}
	return meta
}

func retryReason(wr map[string]any) string {
	if anyx.Bool(wr["replan_suggested"]) {
		return "replan_suggested"
	}
	if anyx.Bool(wr["slot_retry_suggested"]) {
		if anyx.Bool(wr["strong_family_missing"]) {
			return "strong_family_missing"
		}
		if anyx.Bool(wr["evidence_empty"]) {
			return "evidence_empty"
		}
		return "slot_retry_suggested"
	}
	return ""
}

// Ask is a package-level convenience wrapping NewEngine(db).Ask.
func Ask(ctx context.Context, db *sql.DB, in AskInput) AskResult {
	return NewEngine(db).Ask(ctx, in)
}
