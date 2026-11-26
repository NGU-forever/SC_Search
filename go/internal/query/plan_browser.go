package query

import "github.com/frankxiong/sc-search/internal/anyx"

const erkulCalculator = "https://erkul.games/calculator"

func mcpResolveText(hint string, entities []map[string]any) string {
	if hint != "" {
		return hint
	}
	if len(entities) > 0 {
		e := entities[0]
		if zh := anyx.String(e["zh"]); zh != "" {
			return zh
		}
		return anyx.String(e["canonical_name"])
	}
	return ""
}

func browserSearchTerm(entities []map[string]any) string {
	if len(entities) > 0 {
		return anyx.String(entities[0]["canonical_name"])
	}
	return ""
}

func priceSide(q string) string {
	if buyRE.MatchString(q) {
		return "buy"
	}
	if sellRE.MatchString(q) {
		return "sell"
	}
	return "sell"
}

func browserOff() map[string]any {
	return map[string]any{"needed": false}
}

func dedupeResources(rows []map[string]string) []map[string]string {
	seen := map[string]bool{}
	out := []map[string]string{}
	for _, r := range rows {
		url := r["url"]
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		out = append(out, r)
	}
	return out
}

func browserErkul(searchTerm string) map[string]any {
	resources := dedupeResources(GuidesFor("loadout", 3))
	// Prefer community browse as first resource; calculator opens only after confirm.
	start := erkulCalculator
	for _, r := range resources {
		if r["url"] == "https://erkul.games/browse" {
			start = r["url"]
			break
		}
	}
	return map[string]any{
		"needed": true, "site": "erkul", "start_url": start,
		"search_term": searchTerm, "resources": resources, "max_actions": 8,
		"extract":      []any{"community recommended loadout", "selected ship", "DPS summary", "shield / power notes"},
		"do_not":       []any{"open /auth", "/settings", "wander other sites first", "open calculator before community answer unless user confirmed"},
		"wait_confirm": true, "leave_open": true, "handoff_to_user": true,
	}
}

func browserMining(searchTerm string) map[string]any {
	resources := dedupeResources(GuidesFor("mining", 5))
	start := ""
	if len(resources) > 0 {
		start = resources[0]["url"]
	}
	return map[string]any{
		"needed": true, "site": "mining", "start_url": start,
		"search_term": searchTerm, "resources": resources, "max_actions": 8,
		"extract":      []any{"laser / module setup", "instability / resistance notes"},
		"do_not":       []any{"open /auth", "/settings", "wander other sites first"},
		"wait_confirm": true, "leave_open": true, "handoff_to_user": true,
	}
}

func guidePlan(intent, category string, entities []map[string]any, resolveText, reason string, mcpSteps []map[string]any) map[string]any {
	steps := append([]map[string]any{}, mcpSteps...)
	hasResolve := false
	for _, s := range steps {
		if anyx.String(s["tool"]) == "sc_resolve_name" {
			hasResolve = true
			break
		}
	}
	if resolveText != "" && !hasResolve {
		steps = append([]map[string]any{{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}}}, steps...)
	}
	return map[string]any{
		"intent": intent, "path": "mcp_then_browser", "confidence": "medium",
		"entities": entities, "mcp_steps": steps, "browser": BrowserGuide(category, 4),
		"stop_if": "Agent 可用本地证据+推理成文；仅当用户确认或要逐步核对时再开浏览器",
		"reason":  reason, "guide_category": category,
	}
}
