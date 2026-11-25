package query

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	obtainRE         = regexp.MustCompile(`(?i)(哪里买|在哪买|能买到|买得到|多少钱|价格|报价|哪[买卖出]|买点|卖点|获取|怎么得|如何得|掉落|奖励|蓝图|制造|制作|配方|材料|\bbuy\b|\bsell\b|\bprice\b|\bcrafting\b|\bblueprint\b|买)`)
	craftMaterialsRE = regexp.MustCompile(`(?i)(材料|配方|制造|制作|crafting)`)
	// Multi-slot / mechanism questions belong on Router→Worker, not sc_answer bypass.
	answerRouteComplexRE = regexp.MustCompile(`(?i)(手续费|买进|buy\s*in|冷却|cooldown|弃任|abandoned|站点池|site_?pool|有哪些站点|刷法|任务机制|声望要求|敌船|敌舰|lifetime|时效|完整列表|分区站点|pyro\s*region|Region\s*[ABCD])`)
)

var guideIntents = map[string]struct{}{
	"howto": {}, "client_help": {}, "earn_meta": {}, "pledge_meta": {},
	"event_guide": {}, "trade_route": {}, "ship_shop": {},
}

const (
	briefFactLimit  = 5
	fullFactLimit   = 25
	briefPriceLimit = 3
	fullPriceLimit  = 10
	briefDocLimit   = 2
	fullDocLimit    = 5
	relationLimit   = 5
)

const deepOrganizeHint = "有本地证据时：结合 answer_draft 与 evidence 做深度整理，写成更详细的玩家可读说明（结论、分点/小表、版本来源、操作含义）；数值与店名等只能来自证据，勿编造未收录字段。"

// AnswerQuestion is the Go sc_answer implementation (parity with Python answer_question).
func AnswerQuestion(db *sql.DB, question, detailLevel, hintEntity string) (map[string]any, error) {
	level := strings.ToLower(strings.TrimSpace(detailLevel))
	if level != "brief" && level != "full" {
		level = "brief"
	}
	plan, err := QueryPlan(db, question, hintEntity)
	if err != nil {
		return nil, err
	}
	intent, _ := plan["intent"].(string)
	path, _ := plan["path"].(string)
	entities, _ := plan["entities"].([]map[string]any)
	if entities == nil {
		if raw, ok := plan["entities"].([]any); ok {
			entities = castMaps(raw)
		}
	}

	primaryKind := ""
	primary := ""
	if len(entities) > 0 {
		primary = str(entities[0]["canonical_name"])
		primaryKind = str(entities[0]["kind"])
	}

	factLimit := briefFactLimit
	priceLimit := briefPriceLimit
	docLimit := briefDocLimit
	if level == "full" {
		factLimit = fullFactLimit
		priceLimit = fullPriceLimit
		docLimit = fullDocLimit
	}

	live, _ := LiveGameVersion(db)
	label := displayName(entities)

	if shouldDeferAnswerToRoute(question, intent) {
		return answerDeferToRoute(plan, intent, question, live), nil
	}

	if obtainRE.MatchString(question) && primaryKind == "blueprint" && primary != "" {
		return answerBlueprintObtain(db, question, plan, entities, label, primary, live, factLimit)
	}
	if obtainRE.MatchString(question) && (primaryKind == "ship" || primaryKind == "item") && primary != "" && intent == "facts" {
		return answerShopObtain(db, plan, entities, label, primary, primaryKind, live, factLimit, priceLimit)
	}
	if _, ok := guideIntents[intent]; ok {
		return answerGuideIntent(db, plan, entities, label, primary, live)
	}
	if intent == "loadout_calc" || intent == "mining_calc" {
		return answerCalcIntent(db, question, plan, entities, label, primary, live, factLimit)
	}

	evidence := emptyEvidence()
	evidence["entities"] = entities
	followUps := []string{}
	detailAvailable := false
	draft := ""
	missing := []string{}
	hadScoredHit := false

	switch intent {
	case "mission_list", "mission_money":
		faction := factionPrimary(entities)
		fname := primary
		if faction != nil {
			fname = str(faction["canonical_name"])
		}
		list, err := ListMissions(db, fname, "", "", 8)
		if err != nil {
			return nil, err
		}
		missions := []map[string]any{}
		for _, r := range list.Rows {
			missions = append(missions, map[string]any{
				"canonical_name": r["canonical_name"], "title": r["title"],
				"reward_uec": r["reward_uec"], "systems": r["systems"],
				"mission_type": r["mission_type"],
			})
		}
		evidence["missions"] = missions
		if len(list.Rows) > 0 {
			bits := []string{}
			for i, r := range list.Rows {
				if i >= 5 {
					break
				}
				title := str(r["title"])
				if title == "" {
					title = str(r["canonical_name"])
				}
				extra := []string{}
				if r["reward_uec"] != nil {
					extra = append(extra, fmt.Sprintf("%v aUEC", asInt64(r["reward_uec"])))
				}
				if sys := str(r["systems"]); sys != "" {
					extra = append(extra, sys)
				}
				bit := title
				if len(extra) > 0 {
					bit += "（" + strings.Join(extra, "，") + "）"
				}
				bits = append(bits, bit)
			}
			draft = fmt.Sprintf("%s 相关任务示例：%s%s。", orLabel(label, fname), strings.Join(bits, "；"), draftLiveSuffix(live, ""))
			detailAvailable = len(list.Rows) > 5 || list.Truncated
			if detailAvailable {
				followUps = []string{"看更多任务"}
			}
		} else {
			missing = append(missing, "missions")
			draft = fmt.Sprintf("本地未找到 %s 的任务列表。", orLabel(label, fname))
		}
		evidence["missing"] = missing
		conf := recomputeConfidence(intent, str(plan["confidence"]), false, len(entities) > 0, nil, nil, nil, missing)
		return answerOut(intent, path, conf, live, draft, evidence, detailAvailable, followUps, map[string]any{"needed": false}, plan), nil

	case "prices":
		side := "sell"
		if steps, ok := plan["mcp_steps"].([]map[string]any); ok {
			for _, step := range steps {
				if str(step["tool"]) == "sc_best_prices" {
					if args, ok := step["args"].(map[string]any); ok {
						if s := str(args["side"]); s != "" {
							side = s
						}
					}
				}
			}
		} else if stepsAny, ok := plan["mcp_steps"].([]any); ok {
			for _, s := range stepsAny {
				if step, ok := s.(map[string]any); ok && str(step["tool"]) == "sc_best_prices" {
					if args, ok := step["args"].(map[string]any); ok {
						if ss := str(args["side"]); ss != "" {
							side = ss
						}
					}
				}
			}
		}
		terminalName := terminalFromContext(entities, question)
		commodityEntity := firstKind(entities, "commodity")
		commodity := primary
		if commodityEntity != nil {
			commodity = str(commodityEntity["canonical_name"])
		} else if commodity == "" {
			commodity = question
		}
		var rows []map[string]any
		pricesRemain := false
		if terminalName != "" {
			all, err := TerminalPrices(db, terminalName, side)
			if err != nil {
				return nil, err
			}
			if commodityEntity != nil {
				cn := strings.ToLower(str(commodityEntity["canonical_name"]))
				filtered := []map[string]any{}
				for _, r := range all {
					if strings.ToLower(str(r["commodity"])) == cn {
						filtered = append(filtered, r)
					}
				}
				all = filtered
			}
			pricesRemain = len(all) > priceLimit
			if len(all) > priceLimit {
				rows = all[:priceLimit]
			} else {
				rows = all
			}
		} else {
			fetched, err := BestPrices(db, commodity, side, "", priceLimit+1)
			if err != nil {
				return nil, err
			}
			pricesRemain = len(fetched) > priceLimit
			if len(fetched) > priceLimit {
				rows = fetched[:priceLimit]
			} else {
				rows = fetched
			}
		}
		evidence["prices"] = slimPrices(rows)
		if len(rows) > 0 {
			evidence["reasoning_hint"] = deepOrganizeHint
			top := rows[0]
			verSuffix := draftLiveSuffix(live, str(top["game_version"]))
			core := ""
			if terminalName != "" && commodityEntity == nil {
				core = fmt.Sprintf("%s %s 报价示例：%v 约 %v aUEC%s。", terminalName, side, top["commodity"], top["price"], verSuffix)
			} else {
				core = fmt.Sprintf("%v %s 较优：%v 约 %v aUEC%s。", top["commodity"], side, top["terminal"], top["price"], verSuffix)
			}
			draft = "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + core
			detailAvailable = pricesRemain
			if pricesRemain {
				if side == "sell" {
					followUps = []string{"看更多终端", "查买入价"}
				} else {
					followUps = []string{"看更多终端", "查卖出价"}
				}
			}
		} else {
			missing = append(missing, "prices")
			target := terminalName
			if target == "" {
				target = commodity
			}
			draft = fmt.Sprintf("本地没有 %s 的 %s 报价。", target, side)
		}

	case "docs":
		rows, err := SearchDocs(db, question, docLimit)
		if err != nil {
			return nil, err
		}
		docs := []map[string]any{}
		for _, r := range rows {
			snip := str(r["snippet"])
			if snip == "" {
				snip = str(r["body"])
			}
			if len(snip) > 240 {
				snip = snip[:240]
			}
			docs = append(docs, map[string]any{"title": r["title"], "snippet": snip})
		}
		evidence["docs"] = docs
		if len(rows) > 0 {
			snip := str(docs[0]["snippet"])
			if snip != "" {
				if len(snip) > 120 {
					snip = snip[:120]
				}
				draft = "资料摘要：" + snip + "…"
			} else {
				draft = "找到相关文档。"
			}
			detailAvailable = true
			followUps = []string{"展开更多 Wiki 摘要"}
		} else {
			missing = append(missing, "docs")
			draft = "本地 Wiki 摘要未命中。"
		}

	case "live_web":
		draft = "这类信息需要官网或 Spectrum；本地库不覆盖实时 Patch。需要的话再开浏览器。"
		followUps = []string{"打开 RSI 查 Patch Notes"}

	case "live_local":
		if live == "" {
			draft = "本地库尚未记录 Live 版本；请先 sync scmdb。"
			missing = append(missing, "live_version")
		} else {
			draft = fmt.Sprintf("本地对照的当前 Live 版本为 %s。", live)
			followUps = []string{"看各源同步时间", "打开 RSI 查 Patch Notes"}
		}

	case "data_sync":
		rows, err := DataFreshness(db, "")
		if err != nil {
			return nil, err
		}
		annotated, meta := AnnotateFreshnessRows(db, rows)
		evidence["meta"] = meta
		if len(annotated) == 0 {
			draft = "本地没有 ingest 同步记录；请先 sync。"
			missing = append(missing, "ingest_runs")
		} else {
			bits := []string{}
			for i, r := range annotated {
				if i >= 5 {
					break
				}
				src := str(r["source"])
				fin := str(r["finished_at"])
				if src == "" || fin == "" {
					continue
				}
				bits = append(bits, fmt.Sprintf("%s@%s", src, fin))
			}
			ver := live
			if ver == "" {
				ver = str(meta["as_of"])
			}
			if len(bits) == 0 {
				draft = "本地有 ingest 记录但缺少完成时间。"
			} else if ver != "" {
				draft = fmt.Sprintf("本地数据最近同步（对照 Live %s）：%s。", ver, strings.Join(bits, "；"))
			} else {
				draft = fmt.Sprintf("本地数据最近同步：%s。", strings.Join(bits, "；"))
			}
			followUps = []string{"只看 UEX", "只看 SCMDB"}
		}

	case "facts":
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
				allFacts, _ := bundle["facts"].([]map[string]any)
				if allFacts == nil {
					allFacts = castMaps(bundle["facts"])
				}
				picked, scored := pickFacts(allFacts, question, factLimit)
				hadScoredHit = scored
				evidence["facts"] = picked
				srcVer := ""
				if ent, ok := bundle["entity"].(map[string]any); ok {
					srcVer = str(ent["game_version"])
				}
				if scored && len(picked) > 0 {
					evidence["reasoning_hint"] = deepOrganizeHint
					top := picked[0]
					keyLabel := str(top["key_zh"])
					if keyLabel == "" {
						keyLabel = labelFactKey(str(top["key"]))
					}
					core := fmt.Sprintf("%s 的 %s 为 %s%s。", label, keyLabel, factValue(top), draftLiveSuffix(live, srcVer))
					detailAvailable = len(allFacts) > len(picked)
					if detailAvailable {
						core += "需要更细的硬点/部件可以说一声。"
						followUps = []string{"展开全部参数", "只要货舱"}
					}
					draft = "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + core
				} else {
					draft = fmt.Sprintf("本地库有 %s，但未找到匹配字段。", label)
					missing = append(missing, "matching_facts")
				}
			}
		}

	default:
		draft = "本地知识库未覆盖该意图；不会编造数据。"
		missing = append(missing, "intent_handler")
	}

	evidence["missing"] = missing
	if draft == "" {
		draft = "本地知识库未覆盖该问题；不会编造数据。"
	}
	conf := recomputeConfidence(intent, str(plan["confidence"]), hadScoredHit, len(entities) > 0,
		asMaps(evidence["facts"]), asMaps(evidence["prices"]), asMaps(evidence["docs"]), missing)
	browser, _ := plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{"needed": false}
	}
	if needsSoftVerify(intent, conf, hadScoredHit, asMaps(evidence["facts"]), asMaps(evidence["prices"]), asMaps(evidence["docs"])) {
		soft := "要用浏览器核实吗？"
		has := false
		for _, f := range followUps {
			if f == soft {
				has = true
				break
			}
		}
		if !has {
			followUps = append(followUps, soft)
		}
		if intent == "facts" || intent == "prices" || intent == "docs" {
			browser = map[string]any{"needed": false}
		}
	}
	return answerOut(intent, path, conf, live, draft, evidence, detailAvailable, followUps, browser, plan), nil
}

func answerOut(intent, path, conf, live, draft string, evidence map[string]any, detail bool, followUps []string, browser, plan map[string]any) map[string]any {
	return map[string]any{
		"intent": intent, "path": path, "confidence": conf,
		"live_game_version": live, "answer_draft": draft, "evidence": evidence,
		"detail_available": detail, "follow_ups": followUps, "browser": browser, "plan": plan,
	}
}

func shouldDeferAnswerToRoute(question, intent string) bool {
	q := strings.TrimSpace(question)
	if q == "" {
		return false
	}
	if !answerRouteComplexRE.MatchString(q) {
		return false
	}
	// Simple single-commodity price asks stay on sc_answer even if wording overlaps weakly.
	if intent == "prices" && !regexp.MustCompile(`(?i)(手续费|冷却|站点|任务|刷法|声望)`).MatchString(q) {
		return false
	}
	return true
}

// ClassificationContext carries intent, question, and entities for Primary selection.
type ClassificationContext struct {
	Intent   string
	Question string
	Entities []map[string]any
}

// IntentToSuggestedPrimary maps intents without entity/question context (compat).
func IntentToSuggestedPrimary(intent string) string {
	return SuggestedPrimary(ClassificationContext{Intent: intent})
}

// SuggestedPrimary maps QueryPlan intents onto Router primaries with context.
func SuggestedPrimary(ctx ClassificationContext) string {
	intent := strings.TrimSpace(ctx.Intent)
	q := strings.TrimSpace(ctx.Question)
	switch intent {
	case "mission_list":
		if MissionRepContext(q) {
			return "mission_rep"
		}
		return "mission_money"
	case "mission_money":
		return "mission_money"
	case "prices", "trade_route":
		return "trade"
	case "loadout_calc":
		return "loadout"
	case "mining_calc":
		return "mining"
	case "earn_meta":
		return "earn_meta"
	case "howto", "client_help", "pledge_meta", "event_guide":
		return intent
	case "docs":
		return "lore_docs"
	case "facts":
		if len(ctx.Entities) > 0 && str(ctx.Entities[0]["kind"]) == "ship" {
			return "ship_stats"
		}
		return "other"
	case "ship_shop":
		return "ship_buy"
	case "live_web":
		return "patch_live"
	case "live_local", "data_sync":
		return "other"
	default:
		return "other"
	}
}

func intentToSuggestedPrimary(intent string) string {
	return IntentToSuggestedPrimary(intent)
}

func answerDeferToRoute(plan map[string]any, intent, question, live string) map[string]any {
	ents, _ := plan["entities"].([]map[string]any)
	if ents == nil {
		if raw, ok := plan["entities"].([]any); ok {
			ents = castMaps(raw)
		}
	}
	primary := SuggestedPrimary(ClassificationContext{Intent: intent, Question: question, Entities: ents})
	draft := fmt.Sprintf(
		"此题超出 sc_answer 旁路范围（多槽/机制字段），请改用 sc_route → sc_worker_run（建议 primary=%s）。",
		primary,
	)
	evidence := emptyEvidence()
	evidence["missing"] = []string{"use_sc_route"}
	evidence["reasoning_hint"] = "route_required: 用 Router 展开 DAG 后再 Worker 拉 cost/cooldown/site_pool 等证据成文"
	out := answerOut(intent, "router_worker", "low", live, draft, evidence, false,
		[]string{"改用 sc_route"}, map[string]any{"needed": false}, plan)
	out["route_required"] = true
	out["suggested_primary"] = primary
	return out
}

func emptyEvidence() map[string]any {
	return map[string]any{
		"entities": []map[string]any{}, "facts": []map[string]any{}, "relations": []map[string]any{},
		"prices": []map[string]any{}, "docs": []map[string]any{}, "missing": []string{},
	}
}

func displayName(entities []map[string]any) string {
	if len(entities) == 0 {
		return ""
	}
	e := entities[0]
	name := str(e["canonical_name"])
	if zh := str(e["zh"]); zh != "" {
		return fmt.Sprintf("%s（%s）", zh, name)
	}
	return name
}

func orLabel(label, fallback string) string {
	if label != "" {
		return label
	}
	return fallback
}

func draftLiveSuffix(live, sourceVer string) string {
	if live == "" {
		return ""
	}
	src := strings.TrimSpace(sourceVer)
	placeholders := map[string]struct{}{
		"scw-live": {}, "wiki-live": {}, "aliases": {}, "localization-full": {}, "unknown": {},
	}
	if src != "" {
		if _, bad := placeholders[src]; !bad && src != live &&
			!strings.HasPrefix(src, live+"-") && !strings.HasPrefix(src, live+".") {
			return fmt.Sprintf("（Live %s；来源标注 %s）", live, src)
		}
	}
	return fmt.Sprintf("（Live %s）", live)
}

func slimPrices(rows []map[string]any) []map[string]any {
	out := []map[string]any{}
	for _, r := range rows {
		out = append(out, map[string]any{
			"commodity": r["commodity"], "terminal": r["terminal"], "side": r["side"],
			"price": r["price"], "game_version": r["game_version"], "source": r["source"],
		})
	}
	return out
}

func terminalFromContext(entities []map[string]any, question string) string {
	if t := firstKind(entities, "terminal"); t != nil {
		return str(t["canonical_name"])
	}
	_ = question
	return ""
}

func firstKind(entities []map[string]any, kind string) map[string]any {
	for _, e := range entities {
		if str(e["kind"]) == kind {
			return e
		}
	}
	return nil
}

func castMaps(v any) []map[string]any {
	switch t := v.(type) {
	case []map[string]any:
		return t
	case []any:
		out := []map[string]any{}
		for _, x := range t {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func asMaps(v any) []map[string]any { return castMaps(v) }

func labelFactKey(key string) string {
	zh := map[string]string{
		"cargo_capacity": "货舱", "cargo_scu": "货舱", "cargo": "货舱",
		"manufacturer": "制造商", "manufacturer_code": "制造商代码",
		"length": "长度", "beam": "宽度", "height": "高度", "mass": "质量", "mass_total": "质量",
		"size": "尺寸", "type": "类型", "career": "职业", "foci": "定位",
		"production_status": "生产状态", "crew": "乘员", "hp": "结构值",
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return "参数"
	}
	if v, ok := zh[k]; ok {
		return v
	}
	if v, ok := zh[strings.ToLower(k)]; ok {
		return v
	}
	return k
}

func factValue(f map[string]any) string {
	if f["value_num"] != nil {
		unit := str(f["unit"])
		num := f["value_num"]
		if unit != "" {
			return fmt.Sprintf("%v %s", num, unit)
		}
		return fmt.Sprint(num)
	}
	return str(f["value_text"])
}

func pickFacts(facts []map[string]any, question string, limit int) ([]map[string]any, bool) {
	q := question
	ql := strings.ToLower(q)
	wantsCargo := strings.Contains(q, "货舱") || strings.Contains(q, "载货") ||
		strings.Contains(ql, "cargo") || strings.Contains(ql, "scu")
	type scored struct {
		score int
		f     map[string]any
	}
	scoredRows := []scored{}
	cargoKeys := []string{"cargo_capacity", "cargo", "scu", "货舱"}
	zhHints := []struct {
		zh   string
		keys []string
	}{
		{"厂家", []string{"manufacturer"}},
		{"制造商", []string{"manufacturer"}},
		{"长度", []string{"length", "beam", "height"}},
		{"多长", []string{"length"}},
		{"宽", []string{"beam"}},
		{"高", []string{"height"}},
	}
	for _, f := range facts {
		key := strings.ToLower(str(f["key"]))
		score := 0
		if wantsCargo {
			for _, ck := range cargoKeys {
				if strings.Contains(key, ck) || strings.Contains(key, "cargo") {
					score += 10
					break
				}
			}
			if strings.Contains(q, "货舱") || strings.Contains(ql, "cargo") {
				score += 3
			}
		}
		for _, h := range zhHints {
			if strings.Contains(q, h.zh) {
				for _, k := range h.keys {
					if strings.Contains(key, k) {
						score += 8
						break
					}
				}
			}
		}
		if strings.Contains(ql, key) {
			score += 3
		}
		if score > 0 {
			scoredRows = append(scoredRows, scored{score: score, f: f})
		}
	}
	sort.SliceStable(scoredRows, func(i, j int) bool { return scoredRows[i].score > scoredRows[j].score })
	picked := []map[string]any{}
	add := func(f map[string]any) {
		if len(picked) >= limit {
			return
		}
		id := str(f["key"]) + "|" + factValue(f)
		for _, p := range picked {
			if str(p["key"])+"|"+factValue(p) == id {
				return
			}
		}
		sf := map[string]any{
			"key": f["key"], "key_zh": labelFactKey(str(f["key"])),
			"value_text": f["value_text"], "value_num": f["value_num"],
			"unit": f["unit"], "source": f["source"], "game_version": f["game_version"],
		}
		picked = append(picked, sf)
	}
	for _, s := range scoredRows {
		add(s.f)
	}
	had := len(picked) > 0
	for _, f := range facts {
		add(f)
	}
	return picked, had
}

func recomputeConfidence(intent, planConf string, hadScored, hasEntity bool, facts, prices, docs []map[string]any, missing []string) string {
	_ = missing
	switch intent {
	case "facts":
		if hadScored && len(facts) > 0 {
			return "high"
		}
		if hasEntity && len(facts) > 0 {
			return "medium"
		}
		return "low"
	case "prices":
		if len(prices) > 0 {
			return "high"
		}
		return "low"
	case "mission_list":
		if len(missing) == 0 {
			return "high"
		}
		return "low"
	case "docs":
		if len(docs) > 0 {
			return "medium"
		}
		return "low"
	case "live_web":
		return planConf
	default:
		if hasEntity {
			return planConf
		}
		return "low"
	}
}

func needsSoftVerify(intent, conf string, hadScored bool, facts, prices, docs []map[string]any) bool {
	if conf != "low" && conf != "medium" {
		return false
	}
	switch intent {
	case "facts":
		return !hadScored
	case "prices":
		return len(prices) == 0
	case "docs":
		return len(docs) == 0
	default:
		return false
	}
}
