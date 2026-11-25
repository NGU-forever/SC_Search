package orchestrate

import (
	"strings"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/router"
)

// browserGateFor builds host browser policy from path/primary/worker notes.
func browserGateFor(primary, path string, searchTerm string, fromAnswer map[string]any) BrowserGate {
	gate := BrowserGate{}
	if path == "" || path == "mcp_only" {
		return gate
	}
	if path == "browser_only" || path == "mcp_then_browser" {
		gate.Needed = true
		gate.WaitConfirm = true
	}

	if fromAnswer != nil {
		if b, ok := fromAnswer["browser"].(map[string]any); ok {
			gate = mergeBrowser(gate, b)
		}
	}

	switch primary {
	case "loadout":
		gate.Needed = true
		gate.WaitConfirm = true
		gate.LeaveOpen = true
		gate.HandoffToUser = true
		gate.Site = "erkul"
		if gate.StartURL == "" {
			res := query.GuidesFor("loadout", 1)
			if len(res) > 0 {
				gate.StartURL = res[0]["url"]
			}
			if gate.StartURL == "" {
				gate.StartURL = "https://erkul.games/browse"
			}
		}
	case "mining":
		gate.Needed = true
		gate.WaitConfirm = true
		gate.LeaveOpen = true
		gate.HandoffToUser = true
		gate.Site = "mining"
		if gate.StartURL == "" {
			res := query.GuidesFor("mining", 1)
			if len(res) > 0 {
				gate.StartURL = res[0]["url"]
			}
		}
	}

	if searchTerm != "" {
		gate.SearchTerm = searchTerm
	}
	if path == "mcp_only" {
		gate = BrowserGate{}
	}
	return gate
}

func mergeBrowser(base BrowserGate, b map[string]any) BrowserGate {
	if asBool(b["needed"]) {
		base.Needed = true
	}
	if asBool(b["wait_confirm"]) {
		base.WaitConfirm = true
	}
	if asBool(b["leave_open"]) {
		base.LeaveOpen = true
	}
	if asBool(b["handoff_to_user"]) {
		base.HandoffToUser = true
	}
	if s := asString(b["site"]); s != "" {
		base.Site = s
	}
	if s := asString(b["start_url"]); s != "" {
		base.StartURL = s
	}
	if s := asString(b["search_term"]); s != "" {
		base.SearchTerm = s
	}
	if raw, ok := b["resources"].([]map[string]any); ok {
		base.Resources = raw
	} else if raw, ok := b["resources"].([]any); ok {
		out := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		base.Resources = out
	}
	return base
}

func searchTermFromState(st *State) string {
	if canon := canonicalFromEvidence(st.Evidence); canon != "" {
		return canon
	}
	for _, key := range []string{"ship", "commodity", "faction", "item", "blueprint", "laser"} {
		if v := asString(st.Slots[key]); v != "" {
			return v
		}
	}
	return ""
}

func canonicalFromEvidence(evidence []map[string]any) string {
	for _, ev := range evidence {
		if asString(ev["tool"]) != "sc_resolve_name" {
			continue
		}
		rows := asMaps(ev["data"])
		if len(rows) == 0 {
			continue
		}
		if name := asString(rows[0]["canonical_name"]); name != "" {
			return name
		}
	}
	return ""
}

// applyRetry mutates slots/primary for one controlled retry. Returns false if nothing useful changed.
func applyRetry(st *State) bool {
	wr := st.Worker
	if wr == nil {
		return false
	}
	if asBool(wr["replan_suggested"]) && st.ReplanCount < DefaultPolicy().MaxReplan {
		next := replanPrimary(st)
		if next == "" || next == st.Primary {
			st.Warnings = append(st.Warnings, "retry_exhausted: no reliable primary for replan")
			return false
		}
		st.Primary = next
		_, cleaned := router.ValidateRoute(st.Primary, st.Slots)
		st.Slots = cleaned
		st.ReplanCount++
		st.RetryKind = "replan"
		return true
	}
	if asBool(wr["slot_retry_suggested"]) && !asBool(wr["replan_suggested"]) &&
		st.SlotRetryCount < DefaultPolicy().MaxSlotRetry {
		changed := applySlotRetry(st)
		if !changed {
			st.Warnings = append(st.Warnings, "retry_exhausted: no new retrieval term for slot_retry")
			return false
		}
		st.SlotRetryCount++
		st.RetryKind = "slot_retry"
		return true
	}
	return false
}

func replanPrimary(st *State) string {
	if ans := st.Answer; ans != nil {
		if p := asString(ans["suggested_primary"]); p != "" {
			key, _ := router.ValidateRoute(p, st.Slots)
			return key
		}
	}
	if st.PlanIntent != "" {
		return query.IntentToSuggestedPrimary(st.PlanIntent)
	}
	return ""
}

func applySlotRetry(st *State) bool {
	canon := canonicalFromEvidence(st.Evidence)
	if canon == "" {
		canon = searchTermFromState(st)
	}
	if canon == "" {
		return false
	}
	metaWords := map[string]struct{}{
		"开荒": {}, "赚钱": {}, "萌新": {}, "刷钱": {}, "怎么玩": {},
	}
	if _, bad := metaWords[strings.TrimSpace(canon)]; bad {
		return false
	}
	changed := false
	for _, key := range []string{"ship", "commodity", "faction", "item", "blueprint", "laser", "terminal"} {
		cur := asString(st.Slots[key])
		if cur == "" {
			continue
		}
		if cur != canon {
			st.Slots[key] = canon
			changed = true
		}
	}
	if !changed {
		// If no typed slot existed, put canonical into the most likely slot for primary.
		switch st.Primary {
		case "trade", "trade_route":
			if asString(st.Slots["commodity"]) != canon {
				st.Slots["commodity"] = canon
				changed = true
			}
		case "ship_stats", "ship_buy", "loadout", "mining":
			if asString(st.Slots["ship"]) != canon {
				st.Slots["ship"] = canon
				changed = true
			}
		case "mission_rep", "mission_money":
			if asString(st.Slots["faction"]) != canon {
				st.Slots["faction"] = canon
				changed = true
			}
		default:
			if asString(st.Slots["item"]) != canon {
				st.Slots["item"] = canon
				changed = true
			}
		}
	}
	_, cleaned := router.ValidateRoute(st.Primary, st.Slots)
	st.Slots = cleaned
	return changed
}

func routePlanToMap(question string, primary string, slots map[string]any, replanCount int) (map[string]any, error) {
	plan, err := router.ExpandDAG(question, primary, slots, replanCount)
	if err != nil {
		return nil, err
	}
	// worker.RunPlan asserts calls as []any (JSON shape); []map[string]any will not match.
	calls := make([]any, 0, len(plan.Calls))
	for _, c := range plan.Calls {
		calls = append(calls, c)
	}
	return map[string]any{
		"primary":      plan.Primary,
		"slots":        plan.Slots,
		"question":     plan.Question,
		"calls":        calls,
		"path":         plan.Path,
		"notes":        plan.Notes,
		"replan_count": plan.ReplanCount,
		"plan_intent":  plan.PlanIntent,
	}, nil
}
