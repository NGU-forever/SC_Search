package query

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	blueprintReasoning = "蓝图获取/制造：按 evidence.acquisition 组织成文；shop_buyable=unknown 时说明无商店报价、勿断言绝对买不到；有 missions/materials 则引用；刷点/掉率/矿点库无则明说，勿编造。"
	openGuideConfirm   = "要用浏览器打开推荐教程吗？"
)

var guideReasoning = map[string]string{
	"howto":        "操作/找路类：用外链与常识按步骤讲清，勿编造当前补丁独有 UI 细节。",
	"client_help":  "客户端/报错：先列常见排查方向（USER/Reset/换区），具体错误码以教程为准。",
	"earn_meta":    "赚钱开荒：按玩法类型组织建议；可用任务样例推理，勿编造时薪/最优线。",
	"pledge_meta":  "氪金名词：解释 CCU/WB/信用点/退款概念，金额与活动以官网为准。",
	"event_guide":  "活动本：说明玩法框架与风险，逐步流程引用外链，勿编造刷新点。",
	"trade_route":  "跑商航线：可结合本地单价推理；整条最优线指向规划站。",
	"ship_shop":    "船店/租借：本地给静态参数；购点/租金指向 Everark 等外链。",
}

func answerBlueprintObtain(db *sql.DB, question string, plan map[string]any, entities []map[string]any, label, primary, live string, factLimit int) (map[string]any, error) {
	intent, _ := plan["intent"].(string)
	path, _ := plan["path"].(string)
	evidence := emptyEvidence()
	evidence["entities"] = entities

	bundle, err := EntityFacts(db, primary, "full")
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	allFacts := castMaps(bundle["facts"])
	pool := poolNameFromFacts(allFacts)
	missions, err := missionsForPool(db, pool)
	if err != nil {
		return nil, err
	}
	entityRow, _ := bundle["entity"].(map[string]any)
	materials := []map[string]any{}
	if entityRow != nil && entityRow["id"] != nil {
		materials, err = recipeMaterials(db, asInt64(entityRow["id"]))
		if err != nil {
			return nil, err
		}
	}
	wantsMaterials := craftMaterialsRE.MatchString(question)
	acqMissing := []string{}
	if wantsMaterials && len(materials) == 0 {
		acqMissing = append(acqMissing, "crafting_recipe")
	}
	evidence["acquisition"] = acquisitionUnknown(pool, missions, materials, acqMissing, "blueprint_reward", "")
	facts := []map[string]any{}
	for i, f := range allFacts {
		if i >= factLimit {
			break
		}
		facts = append(facts, slimFact(f))
	}
	evidence["facts"] = facts
	evidence["missing"] = acqMissing
	rels := []map[string]any{}
	for i, m := range missions {
		if i >= relationLimit {
			break
		}
		rels = append(rels, map[string]any{
			"rel_type": "mission_reward_blueprint_pool",
			"from_name": m["canonical_name"], "to_name": pool, "qty": nil,
		})
	}
	for i, mat := range materials {
		if i >= relationLimit {
			break
		}
		rels = append(rels, map[string]any{
			"rel_type": "blueprint_requires",
			"from_name": primary, "to_name": mat["name"], "qty": mat["qty"],
		})
	}
	evidence["relations"] = rels
	evidence["reasoning_hint"] = blueprintReasoning
	draft := draftBlueprintScaffold(orLabel(label, primary), pool, live, missions, materials, wantsMaterials, acqMissing)
	return answerOut(intent, path, "high", live, draft, evidence, false, nil, map[string]any{"needed": false}, plan), nil
}

func answerShopObtain(db *sql.DB, plan map[string]any, entities []map[string]any, label, primary, primaryKind, live string, factLimit, priceLimit int) (map[string]any, error) {
	intent, _ := plan["intent"].(string)
	path, _ := plan["path"].(string)
	evidence := emptyEvidence()
	evidence["entities"] = entities

	bundle, err := EntityFacts(db, primary, "full")
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	entityRow, _ := bundle["entity"].(map[string]any)
	srcVer := ""
	if entityRow != nil {
		srcVer = str(entityRow["game_version"])
	}
	buyRows := []map[string]any{}
	if primaryKind == "ship" {
		seenTerms := map[string]bool{}
		for _, ent := range entities {
			if str(ent["kind"]) != "ship" || ent["id"] == nil {
				continue
			}
			rows, err := shipBuyPrices(db, asInt64(ent["id"]), priceLimit)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				term := str(row["terminal"])
				if seenTerms[term] {
					continue
				}
				seenTerms[term] = true
				buyRows = append(buyRows, row)
			}
			if len(buyRows) >= priceLimit {
				break
			}
		}
		sort.SliceStable(buyRows, func(i, j int) bool {
			pi, pj := asFloat(buyRows[i]["price"]), asFloat(buyRows[j]["price"])
			if pi != pj {
				return pi < pj
			}
			return str(buyRows[i]["terminal"]) < str(buyRows[j]["terminal"])
		})
		if len(buyRows) > priceLimit {
			buyRows = buyRows[:priceLimit]
		}
	}
	allFacts := castMaps(bundle["facts"])
	facts := []map[string]any{}
	for i, f := range allFacts {
		if i >= factLimit {
			break
		}
		facts = append(facts, slimFact(f))
	}
	evidence["facts"] = facts
	if len(buyRows) > 0 {
		shops := slimPrices(buyRows)
		evidence["prices"] = shops
		evidence["acquisition"] = acquisitionShopYes(shops, "")
		evidence["missing"] = []string{}
		evidence["reasoning_hint"] = deepOrganizeHint
		return answerOut(intent, path, "high", live, draftShipShop(orLabel(label, primary), buyRows, live), evidence, len(facts) > 0, nil, map[string]any{"needed": false}, plan), nil
	}
	evidence["acquisition"] = acquisitionUnknown("", nil, nil, []string{"shop_prices"}, "shop_unknown", "本地未收录游戏内商店/aUEC 报价")
	evidence["missing"] = []string{"shop_prices"}
	return answerOut(intent, path, "high", live, draftShopUnknown(orLabel(label, primary), live, srcVer), evidence, len(facts) > 0, nil, map[string]any{"needed": false}, plan), nil
}

func answerGuideIntent(db *sql.DB, plan map[string]any, entities []map[string]any, label, primary, live string) (map[string]any, error) {
	intent, _ := plan["intent"].(string)
	path, _ := plan["path"].(string)
	evidence := emptyEvidence()
	evidence["entities"] = entities
	browser, _ := plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{}
	}
	resources := castStringMaps(browser["resources"])
	evidence["guides"] = resources
	evidence["reasoning_hint"] = guideReasoning[intent]
	missionBits := []string{}
	staticBit := ""
	missing := []string{}

	if intent == "earn_meta" {
		list, err := ListMissions(db, "", "", "", 5)
		if err != nil {
			return nil, err
		}
		missions := []map[string]any{}
		for _, r := range list.Rows {
			missions = append(missions, map[string]any{
				"canonical_name": r["canonical_name"], "title": r["title"],
				"reward_uec": r["reward_uec"], "systems": r["systems"],
				"mission_type": r["mission_type"], "contractor": r["contractor"],
			})
		}
		evidence["missions"] = missions
		for i, r := range list.Rows {
			if i >= 4 {
				break
			}
			title := str(r["title"])
			if title == "" {
				title = str(r["canonical_name"])
			}
			bit := title
			if mt := str(r["mission_type"]); mt != "" {
				bit += "[" + mt + "]"
			}
			if r["reward_uec"] != nil {
				bit += fmt.Sprintf("~%daUEC", asInt64(r["reward_uec"]))
			}
			missionBits = append(missionBits, bit)
		}
	}
	if intent == "ship_shop" && primary != "" {
		bundle, err := EntityFacts(db, primary, "full")
		if err != nil {
			return nil, err
		}
		if bundle != nil {
			facts := castMaps(bundle["facts"])
			for _, f := range facts {
				key := str(f["key"])
				if key == "cargo_scu" || key == "cargo_capacity" {
					staticBit = "静态参考：货舱 " + factValue(slimFact(f)) + "。"
					evidence["facts"] = []map[string]any{slimFact(f)}
					break
				}
			}
		}
	}
	if len(resources) == 0 {
		missing = append(missing, "guides")
	}
	draft := draftGuide(intent, orLabel(label, primary), live, resources, missionBits, staticBit)
	evidence["missing"] = missing
	return answerOut(intent, path, "medium", live, draft, evidence, len(resources) > 0 || len(missionBits) > 0, []string{openGuideConfirm}, browser, plan), nil
}

func answerCalcIntent(db *sql.DB, question string, plan map[string]any, entities []map[string]any, label, primary, live string, factLimit int) (map[string]any, error) {
	intent, _ := plan["intent"].(string)
	path, _ := plan["path"].(string)
	evidence := emptyEvidence()
	evidence["entities"] = entities
	followUps := []string{}
	detailAvailable := false
	draft := ""
	missing := []string{}
	hadScored := false

	if primary == "" {
		draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
		missing = append(missing, "entity")
	} else {
		bundle, err := EntityFacts(db, primary, "full")
		if err != nil {
			return nil, err
		}
		if bundle == nil {
			missing = append(missing, "entity_facts")
			draft = fmt.Sprintf("未找到 %s 的本地事实。", orLabel(label, primary))
		} else {
			allFacts := castMaps(bundle["facts"])
			picked, scored := pickFacts(allFacts, question, factLimit)
			hadScored = scored
			evidence["facts"] = picked
			site := "Erkul"
			if intent == "mining_calc" {
				site = "采矿工具站"
			}
			static := ""
			if len(picked) > 0 {
				static = fmt.Sprintf("静态参考：%v=%s。", picked[0]["key"], factValue(picked[0]))
			}
			browser, _ := plan["browser"].(map[string]any)
			guideLinks := formatGuideLinks(castStringMaps(browser["resources"]), 3)
			draft = fmt.Sprintf("%s 配装/计算需 %s。%s本地只能给静态参数；Agent 可先据静态参数推理建议，要精确 DPS/破矿模拟再说，确认后打开工具站；打开并摘要后请把浏览器留在会话里（leave_open），方便用户自己继续改。", orLabel(label, primary), site, static)
			if guideLinks != "" {
				draft += "\n参考：\n" + guideLinks
			}
			followUps = []string{"打开 " + site + " 查配装", "只要静态参数", openGuideConfirm}
			detailAvailable = true
			_ = hadScored
		}
	}
	evidence["missing"] = missing
	browser, _ := plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{"needed": false}
	}
	conf := recomputeConfidence(intent, str(plan["confidence"]), hadScored, len(entities) > 0, asMaps(evidence["facts"]), nil, nil, missing)
	if conf == "" {
		conf = "medium"
	}
	if intent == "loadout_calc" || intent == "mining_calc" {
		if len(asMaps(evidence["facts"])) > 0 {
			conf = "medium"
		} else {
			conf = "low"
		}
	}
	return answerOut(intent, path, conf, live, draft, evidence, detailAvailable, followUps, browser, plan), nil
}

func poolNameFromFacts(facts []map[string]any) string {
	for _, f := range facts {
		if str(f["key"]) == "pool_name" {
			return str(f["value_text"])
		}
	}
	return ""
}

func acquisitionUnknown(pool string, missions, materials []map[string]any, missing []string, mode, hintZH string) map[string]any {
	if hintZH == "" {
		hintZH = "本地仅见蓝图/奖励记录，未找到商店报价"
	}
	if missions == nil {
		missions = []map[string]any{}
	}
	if materials == nil {
		materials = []map[string]any{}
	}
	if missing == nil {
		missing = []string{}
	}
	return map[string]any{
		"mode": mode, "shop_buyable": "unknown", "pool": nullIfEmpty(pool),
		"hint_zh": hintZH, "missions": missions, "materials": materials, "missing": missing,
	}
}

func acquisitionShopYes(shops []map[string]any, hintZH string) map[string]any {
	if hintZH == "" {
		hintZH = "本地有游戏内商店/aUEC 报价"
	}
	return map[string]any{
		"mode": "shop_buy", "shop_buyable": "yes", "pool": nil,
		"hint_zh": hintZH, "missions": []map[string]any{}, "materials": []map[string]any{},
		"shops": shops, "missing": []string{},
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func shipBuyPrices(db *sql.DB, entityID int64, limit int) ([]map[string]any, error) {
	rows, err := db.Query(`
		SELECT
		  c.canonical_name AS commodity,
		  t.canonical_name AS terminal,
		  p.side, p.price, p.scu_available, p.reported_at, p.source, p.game_version
		FROM prices p
		JOIN entities c ON c.id = p.commodity_entity_id
		JOIN entities t ON t.id = p.terminal_entity_id
		WHERE p.commodity_entity_id = ?
		  AND p.side = 'buy'
		  AND p.price > 0
		ORDER BY p.price ASC, t.canonical_name ASC
		LIMIT ?
	`, entityID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
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
		if metaRaw := str(r["meta_json"]); metaRaw != "" {
			var meta map[string]any
			if json.Unmarshal([]byte(metaRaw), &meta) == nil {
				if u := str(meta["unit"]); u != "" {
					unit = u
				}
				if k := str(meta["ingredient_kind"]); k != "" {
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
			alias := str(a["alias"])
			if str(a["lang"]) == "zh" && alias != "" {
				title = alias
				break
			}
		}
		if title == "" {
			for _, a := range aliasRows {
				alias := str(a["alias"])
				if alias != "" && alias != str(r["canonical_name"]) {
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
			title = str(r["canonical_name"])
		}
		out = append(out, map[string]any{
			"canonical_name": r["canonical_name"], "title": title,
			"contractor": fmap["contractor"], "system": fmap["systems"],
		})
	}
	return out, nil
}

func slimFact(f map[string]any) map[string]any {
	key := str(f["key"])
	return map[string]any{
		"key": key, "key_zh": labelFactKey(key),
		"value_text": f["value_text"], "value_num": f["value_num"],
		"unit": f["unit"], "source": f["source"], "game_version": f["game_version"],
	}
}

func draftShopUnknown(label, live, srcVer string) string {
	return fmt.Sprintf("%s 本地知识库未收录游戏内商店/aUEC 报价，无法确认哪里买或多少钱%s。", label, draftLiveSuffix(live, srcVer))
}

func draftShipShop(label string, rows []map[string]any, live string) string {
	if len(rows) == 0 {
		return draftShopUnknown(label, live, "")
	}
	bits := []string{}
	srcVer := ""
	for i, r := range rows {
		if i >= 5 {
			break
		}
		term := str(r["terminal"])
		if term == "" {
			term = "?"
		}
		priceS := "?"
		if r["price"] != nil {
			priceS = fmt.Sprintf("%v", r["price"])
			if f := asFloat(r["price"]); f == float64(int64(f)) {
				priceS = fmt.Sprintf("%d", int64(f))
				// add commas roughly
				priceS = formatIntComma(int64(f))
			}
		}
		bits = append(bits, fmt.Sprintf("%s 约 %s aUEC", term, priceS))
		if srcVer == "" {
			srcVer = str(r["game_version"])
		}
	}
	return "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + label + " 游戏内可购：" + strings.Join(bits, "；") + draftLiveSuffix(live, srcVer) + "。"
}

func formatIntComma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = s[1:]
	}
	var out []byte
	for i, c := range reverseString(s) {
		if i > 0 && i%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	res := reverseString(string(out))
	if n < 0 {
		return "-" + res
	}
	return res
}

func reverseString(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

func formatGuideLinks(resources []map[string]string, limit int) string {
	lines := []string{}
	for i, r := range resources {
		if i >= limit {
			break
		}
		title := r["title"]
		if title == "" {
			title = "链接"
		}
		url := r["url"]
		if url != "" {
			lines = append(lines, "- "+title+"："+url)
		}
	}
	return strings.Join(lines, "\n")
}

func draftGuide(intent, label, live string, resources []map[string]string, missionBits []string, staticBit string) string {
	hint := guideReasoning[intent]
	if hint == "" {
		hint = "结合本地证据与外链推理作答，勿编造库外数值。"
	}
	chunks := []string{"【供 Agent 深度整理】" + hint}
	if label != "" {
		chunks = append(chunks, "相关实体："+label+"。")
	}
	if staticBit != "" {
		chunks = append(chunks, staticBit)
	}
	if len(missionBits) > 0 {
		limit := 4
		if len(missionBits) < limit {
			limit = len(missionBits)
		}
		chunks = append(chunks, "本地任务样例："+strings.Join(missionBits[:limit], "；")+"。")
	}
	switch intent {
	case "earn_meta":
		chunks = append(chunks, "常见开荒方向（需结合版本）：货运/快递熟悉操作 → 低危赏金 → 采矿或大宗货运；收益随补丁波动，以下外链供核对。")
	case "client_help":
		chunks = append(chunks, "常见排查方向：删 USER、换服务器区域、启动器重登、官网页角色 Reset；具体报错以教程为准。")
	case "ship_shop":
		chunks = append(chunks, "本地若无 aUEC 店价，请用下方购点表核对；静态船参数可继续追问。")
	}
	if link := formatGuideLinks(resources, 4); link != "" {
		chunks = append(chunks, "参考外链（引用，勿当作库内事实数值）：\n"+link)
	}
	if live != "" {
		chunks = append(chunks, "版本参考 Live "+live+"。")
	}
	return strings.Join(chunks, "\n")
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
			title := str(m["title"])
			if title == "" {
				title = str(m["canonical_name"])
			}
			meta := []string{}
			if c := str(m["contractor"]); c != "" {
				meta = append(meta, c)
			}
			if s := str(m["system"]); s != "" {
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
			if str(m["ingredient_kind"]) == "resource" {
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
	name := str(m["name"])
	if m["qty"] == nil {
		return name
	}
	qty := m["qty"]
	unit := str(m["unit"])
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

func castStringMaps(v any) []map[string]string {
	out := []map[string]string{}
	switch t := v.(type) {
	case []map[string]string:
		return t
	case []map[string]any:
		for _, m := range t {
			row := map[string]string{}
			for k, vv := range m {
				row[k] = str(vv)
			}
			out = append(out, row)
		}
	case []any:
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				row := map[string]string{}
				for k, vv := range m {
					row[k] = str(vv)
				}
				out = append(out, row)
			}
		}
	}
	return out
}
