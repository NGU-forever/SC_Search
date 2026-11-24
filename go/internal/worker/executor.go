package worker

import (
	"context"
	"sync"
)

// Invoker runs a named MCP-style tool and returns the parsed envelope map.
type Invoker func(ctx context.Context, tool string, args map[string]any) map[string]any

func invoke(handlers Invoker, ctx context.Context, tool string, args map[string]any) map[string]any {
	if handlers == nil {
		return map[string]any{"ok": false, "error": "no invoker", "data": nil, "meta": map[string]any{}}
	}
	parsed := handlers(ctx, tool, args)
	if parsed == nil {
		return map[string]any{"ok": false, "error": "nil tool response", "data": nil, "meta": map[string]any{}}
	}
	return parsed
}

func waves(ctx context.Context, calls []map[string]any, handlers Invoker) ([]map[string]any, map[string]map[string]any) {
	pending := map[string]map[string]any{}
	for _, c := range calls {
		id, _ := c["id"].(string)
		if id == "" {
			continue
		}
		cp := map[string]any{}
		for k, v := range c {
			cp[k] = v
		}
		pending[id] = cp
	}
	results := map[string]map[string]any{}
	ordered := []map[string]any{}

	for len(pending) > 0 {
		ready := []string{}
		for cid, c := range pending {
			if needsSatisfied(c["needs"], results) {
				ready = append(ready, cid)
			}
		}
		if len(ready) == 0 {
			for cid, c := range pending {
				row := map[string]any{
					"id": cid, "tool": c["tool"], "ok": false,
					"error": "unsatisfied needs", "data": nil, "meta": map[string]any{},
					"args": c["args"],
				}
				results[cid] = row
				ordered = append(ordered, row)
			}
			break
		}

		type pair struct {
			cid string
			row map[string]any
		}
		ch := make(chan pair, len(ready))
		var wg sync.WaitGroup
		for _, cid := range ready {
			wg.Add(1)
			go func(cid string) {
				defer wg.Done()
				c := pending[cid]
				tool, _ := c["tool"].(string)
				args, _ := c["args"].(map[string]any)
				if args == nil {
					args = map[string]any{}
				}
				bound := bindArgs(args, results)
				parsed := invoke(handlers, ctx, tool, bound)
				row := map[string]any{
					"id": cid, "tool": tool, "args": bound,
					"ok": parsed["ok"] == true, "error": parsed["error"],
					"data": parsed["data"], "meta": parsed["meta"],
				}
				if row["meta"] == nil {
					row["meta"] = map[string]any{}
				}
				if c["augmented"] == true {
					row["augmented"] = true
				}
				if w, ok := c["evidence_weight"]; ok {
					row["evidence_weight"] = w
				}
				ch <- pair{cid: cid, row: row}
			}(cid)
		}
		wg.Wait()
		close(ch)
		for p := range ch {
			results[p.cid] = p.row
			ordered = append(ordered, p.row)
			delete(pending, p.cid)
		}
	}
	return ordered, results
}

func needsSatisfied(raw any, results map[string]map[string]any) bool {
	switch needs := raw.(type) {
	case nil:
		return true
	case []string:
		for _, ns := range needs {
			if _, have := results[ns]; !have {
				return false
			}
		}
		return true
	case []any:
		for _, n := range needs {
			ns, _ := n.(string)
			if ns == "" {
				continue
			}
			if _, have := results[ns]; !have {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// RunPlan executes a sc_route DAG (parity with Python worker.executor.run_plan).
func RunPlan(ctx context.Context, plan map[string]any, handlers Invoker) map[string]any {
	primary, _ := plan["primary"].(string)
	if primary == "" {
		primary = "other"
	}
	replanCount := 0
	switch v := plan["replan_count"].(type) {
	case float64:
		replanCount = int(v)
	case int:
		replanCount = v
	}
	question, _ := plan["question"].(string)
	rawCalls, _ := plan["calls"].([]any)
	calls := []map[string]any{}
	for _, c := range rawCalls {
		if m, ok := c.(map[string]any); ok {
			calls = append(calls, m)
		}
	}

	callResults, byID := waves(ctx, calls, handlers)
	augmentedCalls := []map[string]any{}
	for i := 0; i < 2; i++ {
		allCalls := append([]map[string]any{}, calls...)
		allCalls = append(allCalls, augmentedCalls...)
		extra := MaybeAugment(allCalls, byID, primary, question)
		if len(extra) == 0 {
			break
		}
		augResults, augByID := waves(ctx, extra, handlers)
		callResults = append(callResults, augResults...)
		for k, v := range augByID {
			byID[k] = v
		}
		augmentedCalls = append(augmentedCalls, extra...)
	}

	evidence := []map[string]any{}
	strongEvidence := []map[string]any{}
	for _, r := range callResults {
		if r["ok"] != true || !dataUseful(r["data"]) {
			continue
		}
		item := map[string]any{
			"id": r["id"], "tool": r["tool"], "ok": r["ok"],
			"data": r["data"], "meta": r["meta"],
		}
		if r["augmented"] == true {
			item["augmented"] = true
		}
		if w, ok := r["evidence_weight"]; ok {
			item["evidence_weight"] = w
		}
		evidence = append(evidence, item)
		if evidenceWeight(r) > 0 {
			strongEvidence = append(strongEvidence, item)
		}
	}

	mismatch := CheckMismatch(primary, callResults)
	empty := EvidenceIsEmpty(strongEvidence)
	strongMissing := StrongFamilyMissing(primary, callResults)
	failed := CollectFailedCalls(callResults)
	suggestReplan := ReplanSuggested(mismatch, replanCount, 1)
	suggestSlots := SlotRetrySuggested(empty, mismatch, replanCount, 1, strongMissing)
	aggMeta := AggregateCallMeta(callResults)

	hint := ""
	if suggestReplan {
		hint = "证据类型与意图明显不符，请改 primary/slots 后重跑（仅一次）"
	} else if suggestSlots {
		hint = "主内容工具无匹配或证据为空，请改 slots/检索词（英文或标准译名）后重跑；勿把开荒/赚钱等口语当 rank_keyword"
	} else if notes, ok := plan["notes"].(string); ok {
		hint = notes
	}

	slots := plan["slots"]
	if slots == nil {
		slots = map[string]any{}
	}
	path, _ := plan["path"].(string)
	if path == "" {
		path = "mcp_only"
	}
	notes, _ := plan["notes"].(string)

	return map[string]any{
		"ok":                    true,
		"primary":               primary,
		"slots":                 slots,
		"question":              question,
		"path":                  path,
		"notes":                 notes,
		"replan_count":          replanCount,
		"evidence":              evidence,
		"call_results":          callResults,
		"augmented_calls":       augmentedCalls,
		"mismatch":              mismatch,
		"replan_suggested":      suggestReplan,
		"evidence_empty":        empty,
		"strong_family_missing": strongMissing,
		"failed_calls":          failed,
		"slot_retry_suggested":  suggestSlots,
		"reasoning_hint":        hint,
		"meta":                  aggMeta,
	}
}

// AggregateCallMeta preserves per-call freshness into a worker-level summary.
func AggregateCallMeta(callResults []map[string]any) map[string]any {
	out := map[string]any{
		"stale":         false,
		"sources":       []any{},
		"game_versions": []any{},
	}
	versions := map[string]struct{}{}
	stale := false
	var asOf any
	for _, r := range callResults {
		meta, _ := r["meta"].(map[string]any)
		if meta == nil {
			continue
		}
		if meta["stale"] == true {
			stale = true
		}
		if asOf == nil {
			if v, ok := meta["as_of"]; ok && v != nil && v != "" {
				asOf = v
			}
		}
		if gv, ok := meta["game_version"]; ok && gv != nil && gv != "" {
			key := strAny(gv)
			if _, seen := versions[key]; !seen {
				versions[key] = struct{}{}
				out["game_versions"] = append(out["game_versions"].([]any), gv)
			}
		}
		if tool, ok := r["tool"].(string); ok && tool != "" {
			out["sources"] = append(out["sources"].([]any), tool)
		}
	}
	out["stale"] = stale
	if asOf != nil {
		out["as_of"] = asOf
	}
	return out
}

func strAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
