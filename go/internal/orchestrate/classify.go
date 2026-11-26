package orchestrate

import (
	"database/sql"
	"strings"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/router"
)

var complexPrimaries = map[string]struct{}{
	"loadout":       {},
	"mining":        {},
	"mission_rep":   {},
	"mission_money": {},
	"earn_meta":     {},
	"trade_route":   {},
	"ship_buy":      {},
	"bug_tip":       {},
	"client_help":   {},
	"howto":         {},
	"pledge_meta":   {},
	"event_guide":   {},
	"patch_live":    {},
}

// classify fills Primary/Slots and decides Answer bypass vs Route.
func classify(db *sql.DB, st *State) (Phase, string, error) {
	plan, err := query.QueryPlan(db, st.Question, "")
	if err != nil {
		return PhaseFailed, "query_plan_error", err
	}
	intent, _ := plan["intent"].(string)
	st.PlanIntent = intent
	ents := asMaps(plan["entities"])
	suggested := query.SuggestedPrimary(query.ClassificationContext{
		Intent: intent, Question: st.Question, Entities: ents,
	})

	explicit := strings.TrimSpace(st.Primary)
	if explicit != "" {
		key, cleaned := router.ValidateRoute(explicit, st.Slots)
		st.Primary = key
		st.Slots = cleaned
	} else {
		st.Primary = suggested
		st.Slots = mergeSlots(st.Slots, slotsFromPlan(st.Primary, plan, st.Question))
		_, cleaned := router.ValidateRoute(st.Primary, st.Slots)
		st.Slots = cleaned
	}

	if shouldBypass(st, intent) {
		st.Bypass = true
		st.Mode = "answer_bypass"
		return PhaseAnswer, "bypass_answer", nil
	}
	st.Bypass = false
	st.Mode = "route_worker"
	return PhaseRoute, "route_worker", nil
}

func shouldBypass(st *State, intent string) bool {
	if _, ok := complexPrimaries[st.Primary]; ok {
		return false
	}
	switch intent {
	case "loadout_calc", "mining_calc", "mission_list", "mission_money", "earn_meta",
		"howto", "client_help", "pledge_meta", "event_guide", "ship_shop", "trade_route":
		return false
	}
	return true
}

func slotsFromPlan(primary string, plan map[string]any, question string) map[string]any {
	out := map[string]any{}
	ents := asMaps(plan["entities"])
	if len(ents) == 0 {
		return out
	}
	name := asString(ents[0]["canonical_name"])
	kind := asString(ents[0]["kind"])
	if name == "" {
		return out
	}
	switch primary {
	case "trade", "trade_route":
		if kind == "commodity" || kind == "" {
			out["commodity"] = name
		}
	case "ship_stats", "ship_buy", "loadout":
		if kind == "ship" || kind == "" {
			out["ship"] = name
		}
	case "mining":
		// Avoid「采矿」alias → ATLS GEO unless the user named that ship.
		if kind == "ship" {
			alias := strings.ToLower(asString(ents[0]["zh"]))
			if alias == "采矿" || alias == "挖矿" {
				break
			}
			out["ship"] = name
		}
	case "mission_rep", "mission_money":
		// Prefer any faction hit, not only ents[0] (short names often match terminals first).
		for _, e := range ents {
			if asString(e["kind"]) == "faction" {
				if n := asString(e["canonical_name"]); n != "" {
					out["faction"] = n
					break
				}
			}
		}
		if out["faction"] == nil && kind == "mission" {
			out["activity"] = name
		}
	case "blueprint", "crafting":
		if kind == "blueprint" || kind == "item" {
			out["item"] = name
			out["blueprint"] = name
		}
	}
	_ = question
	return out
}

func mergeSlots(user, inferred map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range inferred {
		out[k] = v
	}
	for k, v := range user {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func asMaps(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := make([]map[string]any, 0, len(t))
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}
