package worker

var expected = map[string]map[string]struct{}{
	"trade":         set("prices"),
	"trade_route":   set("prices"),
	"ship_stats":    set("facts", "resolve"),
	"ship_buy":      set("facts", "resolve", "docs"),
	"mission_rep":   set("missions", "resolve", "facts"),
	"mission_money": set("missions", "resolve", "facts", "answer"),
	"blueprint":     set("facts", "resolve", "answer"),
	"crafting":      set("facts", "resolve", "answer"),
	"loadout":       set("facts", "resolve", "answer"),
	"mining":        set("facts", "resolve", "answer"),
	"bug_tip":       set("docs", "answer"),
	"client_help":   set("docs"),
	"howto":         set("docs", "answer"),
	"earn_meta":     set("missions", "docs", "answer"),
	"pledge_meta":   set("docs"),
	"event_guide":   set("docs", "freshness"),
	"lore_docs":     set("docs", "answer"),
	"patch_live":    set("freshness"),
	"other":         set("answer", "resolve", "plan", "facts", "docs", "missions", "prices"),
}

var strong = map[string]map[string]struct{}{
	"trade":         set("prices"),
	"trade_route":   set("prices"),
	"ship_stats":    set("facts"),
	"ship_buy":      set("facts"),
	"mission_rep":   set("missions"),
	"mission_money": set("missions"),
	"blueprint":     set("facts"),
	"crafting":      set("facts"),
	"loadout":       set("facts"),
	"mining":        set("facts"),
	"bug_tip":       set("docs"),
	"client_help":   set("docs"),
	"howto":         set("docs", "answer"),
	"earn_meta":     set("missions"),
	"pledge_meta":   set("docs"),
	"event_guide":   set("docs", "freshness"),
	"lore_docs":     set("docs"),
	"patch_live":    set("freshness"),
	"other":         set(),
}

var toolFamily = map[string]string{
	"sc_best_prices":     "prices",
	"sc_terminal_prices": "prices",
	"sc_entity_facts":    "facts",
	"sc_resolve_name":    "resolve",
	"sc_list_missions":   "missions",
	"sc_locations":       "locations",
	"sc_search_docs":     "docs",
	"sc_data_freshness":  "freshness",
	"sc_answer":          "answer",
	"sc_query_plan":      "plan",
}

var weakAlone = set("docs", "freshness", "plan")

func set(xs ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func dataUseful(data any) bool {
	if data == nil {
		return false
	}
	switch t := data.(type) {
	case []any:
		return len(t) > 0
	case []map[string]any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	case string:
		return t != ""
	default:
		return true
	}
}

func evidenceWeight(row map[string]any) int {
	v, ok := row["evidence_weight"]
	if !ok {
		return 1
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 1
	}
}

func familiesPresent(callResults []map[string]any) map[string]struct{} {
	found := map[string]struct{}{}
	for _, row := range callResults {
		if row["ok"] != true {
			continue
		}
		if evidenceWeight(row) <= 0 {
			continue
		}
		if !dataUseful(row["data"]) {
			continue
		}
		tool, _ := row["tool"].(string)
		if fam, ok := toolFamily[tool]; ok {
			found[fam] = struct{}{}
		}
	}
	return found
}

func intersect(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}

func subset(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

func StrongFamilyMissing(primary string, callResults []map[string]any) bool {
	s := strong[primary]
	if s == nil {
		s = strong["other"]
	}
	if len(s) == 0 {
		return false
	}
	present := familiesPresent(callResults)
	return !intersect(present, s)
}

func CheckMismatch(primary string, callResults []map[string]any) bool {
	exp := expected[primary]
	if exp == nil {
		exp = expected["other"]
	}
	present := familiesPresent(callResults)
	if len(present) == 0 {
		return false
	}
	if intersect(present, exp) {
		strongExp := map[string]struct{}{}
		for k := range exp {
			if _, weak := weakAlone[k]; !weak {
				strongExp[k] = struct{}{}
			}
		}
		if len(strongExp) > 0 && !intersect(present, strongExp) && subset(present, weakAlone) {
			return true
		}
		return false
	}
	return true
}

func ReplanSuggested(mismatch bool, replanCount, limit int) bool {
	return mismatch && replanCount < limit
}

func EvidenceIsEmpty(evidence []map[string]any) bool {
	return len(evidence) == 0
}

func SlotRetrySuggested(evidenceEmpty, mismatch bool, replanCount, limit int, strongMissing bool) bool {
	if mismatch {
		return false
	}
	if replanCount >= limit {
		return false
	}
	return evidenceEmpty || strongMissing
}

func CollectFailedCalls(callResults []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, row := range callResults {
		tool := row["tool"]
		args := row["args"]
		if args == nil {
			args = map[string]any{}
		}
		meta, _ := row["meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		if row["ok"] != true {
			out = append(out, map[string]any{
				"tool": tool, "args": args, "reason": firstNonNil(row["error"], "error"),
			})
			continue
		}
		if meta["why"] == "no_match" {
			out = append(out, map[string]any{
				"tool": tool, "args": args, "reason": firstNonNil(meta["message"], "no_match"),
			})
		}
	}
	return out
}

func firstNonNil(a, b any) any {
	if a == nil || a == "" {
		return b
	}
	return a
}
