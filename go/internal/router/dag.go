package router

import (
	"strconv"
	"strings"
)

var skipInWorker = map[string]struct{}{"sc_answer": {}}

var needsResolve = map[string]struct{}{
	"sc_entity_facts":    {},
	"sc_list_missions":   {},
	"sc_best_prices":     {},
	"sc_terminal_prices": {},
}

var resolveSlotKeys = []string{
	"faction", "ship", "commodity", "item", "blueprint", "laser", "terminal",
}

// RoutePlan is the sc_route data payload.
type RoutePlan struct {
	Primary     string           `json:"primary"`
	Slots       map[string]any   `json:"slots"`
	Question    string           `json:"question"`
	Calls       []map[string]any `json:"calls"`
	Path        string           `json:"path"`
	Notes       string           `json:"notes"`
	ReplanCount int              `json:"replan_count"`
	PlanIntent  any              `json:"plan_intent"`
}

func ValidateRoute(primary string, slots map[string]any) (string, map[string]any) {
	key := strings.TrimSpace(primary)
	pbs, _ := loadPlaybooks()
	if _, ok := pbs[key]; !ok {
		key = "other"
	}
	pb := pbs[key]
	allowed := map[string]struct{}{}
	if raw, ok := pb["slots"].([]any); ok {
		for _, s := range raw {
			if ss, ok := s.(string); ok {
				allowed[ss] = struct{}{}
			}
		}
	}
	cleaned := map[string]any{}
	for k, v := range slots {
		if _, ok := allowed[k]; !ok {
			continue
		}
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			cleaned[k] = s
			continue
		}
		cleaned[k] = v
	}
	return key, cleaned
}

func ExpandDAG(question, primary string, slots map[string]any, replanCount int) (RoutePlan, error) {
	if slots == nil {
		slots = map[string]any{}
	}
	key, cleaned := ValidateRoute(primary, slots)
	pb := playbookFor(key)
	if pb == nil {
		pb = playbookFor("other")
	}
	tools := filterTools(asStringSlice(pb["tools"]), true)
	if len(tools) == 0 {
		tools = filterTools(asStringSlice(pb["tools"]), false)
		if len(tools) == 0 {
			tools = []string{"sc_resolve_name"}
		}
	}

	hasResolve := contains(tools, "sc_resolve_name")
	resolveID := ""
	if hasResolve {
		resolveID = "r1"
	}
	if hasResolve && resolveText(cleaned, question) == "" {
		tools = remove(tools, "sc_resolve_name")
		hasResolve = false
		resolveID = ""
	}
	if key == "trade" && cleaned["terminal"] == nil {
		tools = remove(tools, "sc_terminal_prices")
	}
	calls := []map[string]any{}
	seq := 0
	for _, tool := range tools {
		seq++
		cid := "c" + strconv.Itoa(seq)
		if tool == "sc_resolve_name" {
			cid = "r1"
		}
		needs := []string{}
		useResolve := ""
		if _, ok := needsResolve[tool]; ok && resolveID != "" && tool != "sc_resolve_name" {
			needs = []string{resolveID}
			useResolve = resolveID
		}
		args := argsFor(tool, cleaned, question, useResolve)
		// Never dump the global mission catalog for money/rep questions without a faction.
		if tool == "sc_list_missions" && cleaned["faction"] == nil &&
			(key == "mission_money" || key == "mission_rep") {
			continue
		}
		if tool == "sc_list_missions" && cleaned["faction"] == nil {
			needs = []string{}
			args = argsFor(tool, cleaned, question, "")
		}
		if tool == "sc_list_missions" && key == "mission_money" {
			args["prefer_fee"] = true
			// Large factions (e.g. Shubin ~180) need a higher cap so fee/reward ranges are complete.
			if args["limit"] == nil {
				args["limit"] = 250
			}
		}
		if tool == "sc_list_missions" && key == "earn_meta" &&
			args["faction"] == nil && args["rank_keyword"] == nil && args["system"] == nil {
			continue
		}
		calls = append(calls, map[string]any{
			"id": cid, "tool": tool, "args": args, "needs": needs,
		})
	}
	if replanCount < 0 {
		replanCount = 0
	}
	return RoutePlan{
		Primary:     key,
		Slots:       cleaned,
		Question:    question,
		Calls:       calls,
		Path:        strOr(pb["path"], "mcp_only"),
		Notes:       strOr(pb["notes"], ""),
		ReplanCount: replanCount,
		PlanIntent:  pb["plan_intent"],
	}, nil
}

func filterTools(tools []string, skipAnswer bool) []string {
	out := []string{}
	for _, t := range tools {
		if skipAnswer {
			if _, skip := skipInWorker[t]; skip {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func resolveText(slots map[string]any, question string) string {
	for _, key := range resolveSlotKeys {
		if v, ok := slots[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(question)
}

func argsFor(tool string, slots map[string]any, question, resolveID string) map[string]any {
	switch tool {
	case "sc_resolve_name":
		return map[string]any{"text": resolveText(slots, question)}
	case "sc_entity_facts":
		if resolveID != "" {
			return map[string]any{"name_or_id": "$" + resolveID + ".canonical"}
		}
		return map[string]any{"name_or_id": resolveText(slots, question)}
	case "sc_list_missions":
		args := map[string]any{}
		if resolveID != "" && slots["faction"] != nil {
			args["faction"] = "$" + resolveID + ".canonical"
		} else if slots["faction"] != nil {
			args["faction"] = slots["faction"]
		}
		rank := missionRankKeyword(asString(slots["rank"]), asString(slots["rank_keyword"]), asString(slots["activity"]))
		if rank != "" {
			args["rank_keyword"] = rank
		}
		if slots["system"] != nil {
			args["system"] = slots["system"]
		}
		return args
	case "sc_best_prices":
		commodity := asString(slots["commodity"])
		if commodity == "" {
			commodity = resolveText(slots, question)
		}
		if resolveID != "" {
			commodity = "$" + resolveID + ".canonical"
		}
		args := map[string]any{
			"commodity": commodity,
			"side":      firstNonEmpty(asString(slots["side"]), "sell"),
		}
		if slots["system"] != nil {
			args["system"] = slots["system"]
		}
		return args
	case "sc_terminal_prices":
		terminal := asString(slots["terminal"])
		if terminal == "" {
			terminal = resolveText(slots, question)
		}
		if resolveID != "" && slots["terminal"] != nil {
			terminal = "$" + resolveID + ".canonical"
		}
		args := map[string]any{"terminal": terminal}
		if slots["side"] != nil {
			args["side"] = slots["side"]
		}
		return args
	case "sc_search_docs":
		q := ""
		for _, key := range []string{"topic", "query", "event", "activity", "goal"} {
			if v := asString(slots[key]); v != "" {
				q = docsQuery(v, question)
				break
			}
		}
		if q == "" {
			q = docsQuery("", question)
		}
		return map[string]any{"query": q}
	case "sc_data_freshness":
		return map[string]any{}
	case "sc_query_plan":
		args := map[string]any{"question": question}
		hint := resolveText(slots, question)
		if hint != "" && hint != question {
			args["hint_entity"] = hint
		}
		return args
	default:
		return map[string]any{}
	}
}

func asStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func strOr(v any, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func remove(xs []string, drop string) []string {
	out := []string{}
	for _, x := range xs {
		if x != drop {
			out = append(out, x)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
