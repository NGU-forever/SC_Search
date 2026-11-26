package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

const blueprintReasoning = "蓝图获取/制造：按 evidence.acquisition 组织成文；shop_buyable=unknown 时说明无商店报价、勿断言绝对买不到；有 missions/materials 则引用；刷点/掉率/矿点库无则明说，勿编造。"

func answerBlueprintObtain(c *answerContext) (map[string]any, error) {
	ev := c.evidence()

	bundle, err := EntityFacts(c.DB, c.Primary, "full")
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	allFacts := anyx.Maps(bundle["facts"])
	pool := poolNameFromFacts(allFacts)
	missions, err := missionsForPool(c.DB, pool)
	if err != nil {
		return nil, err
	}
	entityRow, _ := bundle["entity"].(map[string]any)
	materials := []map[string]any{}
	if entityRow != nil && entityRow["id"] != nil {
		materials, err = recipeMaterials(c.DB, asInt64(entityRow["id"]))
		if err != nil {
			return nil, err
		}
	}
	wantsMaterials := craftMaterialsRE.MatchString(c.Question)
	acqMissing := []string{}
	if wantsMaterials && len(materials) == 0 {
		acqMissing = append(acqMissing, "crafting_recipe")
	}
	ev["acquisition"] = acquisitionUnknown(pool, missions, materials, acqMissing, "blueprint_reward", "")
	facts := []map[string]any{}
	for i, f := range allFacts {
		if i >= c.FactLimit {
			break
		}
		facts = append(facts, slimFact(f))
	}
	ev["facts"] = facts
	ev["missing"] = acqMissing
	rels := []map[string]any{}
	for i, m := range missions {
		if i >= relationLimit {
			break
		}
		rels = append(rels, map[string]any{
			"rel_type":  "mission_reward_blueprint_pool",
			"from_name": m["canonical_name"], "to_name": pool, "qty": nil,
		})
	}
	for i, mat := range materials {
		if i >= relationLimit {
			break
		}
		rels = append(rels, map[string]any{
			"rel_type":  "blueprint_requires",
			"from_name": c.Primary, "to_name": mat["name"], "qty": mat["qty"],
		})
	}
	ev["relations"] = rels
	ev["reasoning_hint"] = blueprintReasoning
	draft := draftBlueprintScaffold(orLabel(c.Label, c.Primary), pool, c.Live, missions, materials, wantsMaterials, acqMissing)
	return answerOut(c.Intent, c.Path, "high", c.Live, draft, ev, false, nil, map[string]any{"needed": false}, c.Plan), nil
}

func poolNameFromFacts(facts []map[string]any) string {
	for _, f := range facts {
		if anyx.String(f["key"]) == "pool_name" {
			return anyx.String(f["value_text"])
		}
	}
	return ""
}

func recipeMaterials(db *sql.DB, blueprintEntityID int64) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT te.canonical_name AS name, rel.qty, rel.meta_json
		FROM relations rel
		JOIN entities te ON te.id = rel.to_id
		WHERE rel.from_id = ?
		  AND rel.rel_type = 'blueprint_requires'
		ORDER BY te.canonical_name
	`, blueprintEntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	raw, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range raw {
		unit := "SCU"
		ingredientKind := "resource"
		if metaRaw := anyx.String(r["meta_json"]); metaRaw != "" {
			var meta map[string]any
			if json.Unmarshal([]byte(metaRaw), &meta) == nil {
				if u := anyx.String(meta["unit"]); u != "" {
					unit = u
				}
				if k := anyx.String(meta["ingredient_kind"]); k != "" {
					ingredientKind = k
				}
			}
		}
		out = append(out, map[string]any{
			"name": r["name"], "qty": r["qty"], "unit": unit, "ingredient_kind": ingredientKind,
		})
	}
	return out, nil
}

func missionsForPool(db *sql.DB, pool string) ([]map[string]any, error) {
	if pool == "" {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT m.id, m.canonical_name
		FROM entities pool
		JOIN relations rel
		  ON rel.to_id = pool.id
		 AND rel.rel_type = 'mission_reward_blueprint_pool'
		JOIN entities m ON m.id = rel.from_id AND m.kind = 'mission'
		WHERE pool.canonical_name = ? COLLATE NOCASE
		LIMIT 5
	`, pool)
	if err != nil {
		return nil, err
	}
	items, err := scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, r := range items {
		mid := asInt64(r["id"])
		aliases, err := db.Query(`
			SELECT alias, lang FROM aliases WHERE entity_id = ?
			ORDER BY CASE lang WHEN 'zh' THEN 0 WHEN 'en' THEN 1 ELSE 2 END, length(alias) DESC
		`, mid)
		if err != nil {
			return nil, err
		}
		aliasRows, err := scanMaps(aliases)
		aliases.Close()
		if err != nil {
			return nil, err
		}
		title := ""
		for _, a := range aliasRows {
			alias := anyx.String(a["alias"])
			if anyx.String(a["lang"]) == "zh" && alias != "" {
				title = alias
				break
			}
		}
		if title == "" {
			for _, a := range aliasRows {
				alias := anyx.String(a["alias"])
				if alias != "" && alias != anyx.String(r["canonical_name"]) {
					title = alias
					break
				}
			}
		}
		factRows, err := db.Query(`
			SELECT key, value_text FROM facts
			WHERE entity_id = ? AND key IN ('contractor', 'systems')
		`, mid)
		if err != nil {
			return nil, err
		}
		fmap := map[string]string{}
		for factRows.Next() {
			var k, v sql.NullString
			if err := factRows.Scan(&k, &v); err != nil {
				factRows.Close()
				return nil, err
			}
			fmap[k.String] = v.String
		}
		factRows.Close()
		if title == "" {
			title = anyx.String(r["canonical_name"])
		}
		out = append(out, map[string]any{
			"canonical_name": r["canonical_name"], "title": title,
			"contractor": fmap["contractor"], "system": fmap["systems"],
		})
	}
	return out, nil
}

func draftBlueprintScaffold(label, pool, live string, missions, materials []map[string]any, wantsMaterials bool, missing []string) string {
	chunks := []string{"【供 Agent 深度整理】" + blueprintReasoning}
	if label != "" {
		chunks = append(chunks, "实体："+label+"。")
	}
	chunks = append(chunks, "商店报价：本地未找到（shop_buyable=unknown）。")
	if pool != "" {
		chunks = append(chunks, "奖励池："+pool+"。")
	}
	if len(missions) > 0 {
		bits := []string{}
		for i, m := range missions {
			if i >= 5 {
				break
			}
			title := anyx.String(m["title"])
			if title == "" {
				title = anyx.String(m["canonical_name"])
			}
			meta := []string{}
			if c := anyx.String(m["contractor"]); c != "" {
				meta = append(meta, c)
			}
			if s := anyx.String(m["system"]); s != "" {
				meta = append(meta, s)
			}
			bit := title
			if len(meta) > 0 {
				bit += "（" + strings.Join(meta, "，") + "）"
			}
			bits = append(bits, bit)
		}
		chunks = append(chunks, "任务线索："+strings.Join(bits, "；")+"。")
	}
	if len(materials) > 0 {
		parts := []string{}
		hasResource := false
		for _, m := range materials {
			parts = append(parts, formatMaterialLine(m))
			if anyx.String(m["ingredient_kind"]) == "resource" {
				hasResource = true
			}
		}
		chunks = append(chunks, "制造材料："+strings.Join(parts, "、")+"。")
		if hasResource {
			chunks = append(chunks, "资源类刷点/矿点：库未收录。")
		}
	}
	miss := map[string]bool{}
	for _, m := range missing {
		miss[m] = true
	}
	if wantsMaterials && miss["crafting_recipe"] {
		chunks = append(chunks, "库无制造配方/材料来源。")
	}
	if live != "" {
		chunks = append(chunks, "版本参考 Live "+live+"。")
	}
	return strings.Join(chunks, "\n")
}

func formatMaterialLine(m map[string]any) string {
	name := anyx.String(m["name"])
	if m["qty"] == nil {
		return name
	}
	qty := m["qty"]
	unit := anyx.String(m["unit"])
	switch unit {
	case "SCU":
		return fmt.Sprintf("%s %v SCU", name, qty)
	case "count":
		return fmt.Sprintf("%s ×%v", name, qty)
	default:
		if unit != "" {
			return fmt.Sprintf("%s %v %s", name, qty, unit)
		}
		return fmt.Sprintf("%s %v", name, qty)
	}
}
