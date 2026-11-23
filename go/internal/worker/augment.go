package worker

var augmentFacts = map[string]struct{}{"sc_entity_facts": {}}
var emptyAugmentPrimaries = map[string]struct{}{"earn_meta": {}, "howto": {}, "bug_tip": {}}
var emptyAugmentTools = []string{"sc_search_docs", "sc_answer"}

func hasOKData(results map[string]map[string]any, tool string) bool {
	for _, row := range results {
		if row["tool"] != tool || row["ok"] != true {
			continue
		}
		if dataUseful(row["data"]) {
			return true
		}
	}
	return false
}

func MaybeAugment(calls []map[string]any, results map[string]map[string]any, primary, question string) []map[string]any {
	existingTools := map[string]struct{}{}
	existingIDs := map[string]struct{}{}
	for _, c := range calls {
		if t, ok := c["tool"].(string); ok {
			existingTools[t] = struct{}{}
		}
		if id, ok := c["id"].(string); ok {
			existingIDs[id] = struct{}{}
		}
	}
	for id := range results {
		existingIDs[id] = struct{}{}
	}
	extra := []map[string]any{}

	if _, has := existingTools["sc_entity_facts"]; !has {
		for _, c := range calls {
			if c["tool"] != "sc_resolve_name" {
				continue
			}
			rid, _ := c["id"].(string)
			row := results[rid]
			if row == nil || row["ok"] != true {
				continue
			}
			canon := canonicalFromResolve(row["data"])
			if canon == "" {
				continue
			}
			cid := "aug_facts_" + rid
			if _, ok := existingIDs[cid]; ok {
				continue
			}
			if _, ok := augmentFacts["sc_entity_facts"]; !ok {
				continue
			}
			extra = append(extra, map[string]any{
				"id": cid, "tool": "sc_entity_facts",
				"args": map[string]any{"name_or_id": canon},
				"needs": []any{}, "augmented": true,
			})
			existingIDs[cid] = struct{}{}
		}
	}

	key := primary
	q := question
	if _, ok := emptyAugmentPrimaries[key]; ok && q != "" {
		strongOK := false
		for _, t := range []string{"sc_list_missions", "sc_search_docs", "sc_answer", "sc_entity_facts"} {
			if hasOKData(results, t) {
				strongOK = true
				break
			}
		}
		tried := map[string]struct{}{}
		for t := range existingTools {
			tried[t] = struct{}{}
		}
		for _, row := range results {
			if t, ok := row["tool"].(string); ok {
				tried[t] = struct{}{}
			}
		}
		if !strongOK {
			for _, tool := range emptyAugmentTools {
				if _, ok := tried[tool]; ok {
					continue
				}
				cid := "aug_empty_" + tool
				if _, ok := existingIDs[cid]; ok {
					continue
				}
				args := map[string]any{}
				if tool == "sc_search_docs" {
					args["query"] = q
				} else {
					args["question"] = q
				}
				extra = append(extra, map[string]any{
					"id": cid, "tool": tool, "args": args,
					"needs": []any{}, "augmented": true,
				})
				existingIDs[cid] = struct{}{}
			}
		}
	}
	return extra
}
