package query

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

var compactFactKeys = map[string]struct{}{
	"reward_uec": {}, "cost_uec": {}, "cooldown": {}, "abandoned_cooldown": {},
	"lifetime": {}, "reputation_min": {}, "reputation_rank": {}, "reputation_max": {},
	"reputation_rank_max": {}, "pyro_regions": {}, "site_pool": {}, "site_pool_by_region": {},
	"enemy_min": {}, "enemy_max": {}, "mission_type": {}, "category": {}, "contractor": {}, "systems": {},
	"illegal": {}, "cargo_capacity": {}, "manufacturer": {}, "size": {},
	"location_type": {}, "system": {}, "planet": {}, "moon": {},
	"cooldown_unit": {}, "lifetime_unit": {}, "region_tag": {}, "difficulty_band": {}, "debug_name": {},
}

var entityPriority = map[string]int{
	"ship": 0, "mission": 1, "blueprint": 2, "blueprint_pool": 3,
	"commodity": 4, "item": 5, "faction": 6,
}

func EntityFacts(db *sql.DB, nameOrID string, detail string) (map[string]any, error) {
	text := strings.TrimSpace(nameOrID)
	if text == "" {
		return nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(detail))
	if mode != "compact" && mode != "full" {
		// P0: default compact so faction/location dumps don't blow LLM context.
		mode = "compact"
	}

	var entity map[string]any
	if _, err := strconv.Atoi(text); err == nil {
		rows, err := db.Query(`
			SELECT id, kind, canonical_name, slug, uuid, game_version
			FROM entities WHERE id = ?
		`, text)
		if err != nil {
			return nil, err
		}
		items, err := scanMaps(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			entity = items[0]
		}
	}
	if entity == nil {
		matches, err := ResolveName(db, text)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, nil
		}
		sort.SliceStable(matches, func(i, j int) bool {
			pi, okI := entityPriority[str(matches[i]["kind"])]
			if !okI {
				pi = 99
			}
			pj, okJ := entityPriority[str(matches[j]["kind"])]
			if !okJ {
				pj = 99
			}
			return pi < pj
		})
		m0 := matches[0]
		entity = map[string]any{
			"id":             m0["id"],
			"kind":           m0["kind"],
			"canonical_name": m0["canonical_name"],
			"slug":           m0["slug"],
			"uuid":           m0["uuid"],
			"game_version":   m0["game_version"],
		}
	}

	entityID := asInt64(entity["id"])
	factRows, err := db.Query(`
		SELECT key, value_text, value_num, unit, source, game_version
		FROM facts WHERE entity_id = ?
		ORDER BY key
	`, entityID)
	if err != nil {
		return nil, err
	}
	facts, err := scanMaps(factRows)
	factRows.Close()
	if err != nil {
		return nil, err
	}

	aliasRows, err := db.Query(`
		SELECT alias, lang, source FROM aliases WHERE entity_id = ?
		ORDER BY alias
	`, entityID)
	if err != nil {
		return nil, err
	}
	aliases, err := scanMaps(aliasRows)
	aliasRows.Close()
	if err != nil {
		return nil, err
	}

	relRows, err := db.Query(`
		SELECT rel.id, rel.rel_type, rel.qty, rel.meta_json, rel.source, rel.game_version,
		       rel.from_id, rel.to_id,
		       fe.canonical_name AS from_name, fe.kind AS from_kind,
		       te.canonical_name AS to_name, te.kind AS to_kind
		FROM relations rel
		JOIN entities fe ON fe.id = rel.from_id
		JOIN entities te ON te.id = rel.to_id
		WHERE rel.from_id = ? OR rel.to_id = ?
		ORDER BY rel.rel_type, te.canonical_name
	`, entityID, entityID)
	if err != nil {
		return nil, err
	}
	relMaps, err := scanMaps(relRows)
	relRows.Close()
	if err != nil {
		return nil, err
	}
	relations := make([]map[string]any, 0, len(relMaps))
	for _, item := range relMaps {
		meta := item["meta_json"]
		delete(item, "meta_json")
		if meta != nil && str(meta) != "" {
			var parsed any
			if err := json.Unmarshal([]byte(str(meta)), &parsed); err == nil {
				item["meta"] = parsed
			} else {
				item["meta"] = meta
			}
		}
		relations = append(relations, item)
	}

	if mode == "compact" {
		filtered := make([]map[string]any, 0)
		for _, f := range facts {
			if _, ok := compactFactKeys[str(f["key"])]; ok {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			if len(facts) > 20 {
				facts = facts[:20]
			}
		} else {
			facts = filtered
		}
		if len(aliases) > 8 {
			aliases = aliases[:8]
		}
		if len(relations) > 20 {
			relations = relations[:20]
		}
	}

	return map[string]any{
		"entity":    entity,
		"aliases":   aliases,
		"facts":     facts,
		"relations": relations,
		"detail":    mode,
	}, nil
}
