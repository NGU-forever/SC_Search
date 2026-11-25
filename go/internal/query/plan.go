package query

import (
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const erkulCalculator = "https://erkul.games/calculator"

var (
	loadoutRE     = regexp.MustCompile(`(?i)(dps|loadout|erkul|配装|怎么配|如何配|打炮|火力|护盾续航)`)
	miningRE      = regexp.MustCompile(`(?i)(破矿|精炼模拟|regolith|激光头|采矿头|mining\s*laser)`)
	pricesRE      = regexp.MustCompile(`(?i)(价格|报价|最低价|最高价|最便宜|最贵|哪里买|哪里卖|在哪买|在哪卖|哪[买卖出]|买点|卖点|买入|卖出|多少钱|\bbuy\b|\bsell\b|price)`)
	missionListRE = regexp.MustCompile(`(?i)(有哪些任务|哪些任务|任务列表|什么任务|刷什么任务|任务有哪些)`)
	clientRE      = regexp.MustCompile(`(?i)(报错|进不去|打不开|锁服|汉化|虚拟内存|鼠标失灵|19000|32212|删\s*user|角色重置|reset\b|ptu怎么进|eptu|启动器)`)
	howtoRE       = regexp.MustCompile(`(?i)(怎么走|如何走|叫船|机库|床上?登出|按键|键位|跃迁怎么|量子怎么|怎么起飞|怎么降落|对接|找路|路线|操作教程|新手教程|怎么上(我的)?飞?船|复活点|出生点)`)
	earnMetaRE    = regexp.MustCompile(`(?i)(怎么赚钱|如何赚钱|怎么刷钱|开荒|萌新(该|要)?干什么|新手(该|要)?干什么|第一桶金|赚钱方法|推荐玩法|怎么玩)`)
	pledgeRE      = regexp.MustCompile(`(?i)(ccu|warbond|\bwb\b|融船|信用点|store\s*credit|叠叠乐|退款|灰市|star\s*hangar|资格包|新手包|邀请码|lti)`)
	eventRE       = regexp.MustCompile(`(?i)(890\s*登舰|奥里森之围|海盗潮|iae|free\s*fly|免费试飞|活动怎么|北极星兑换|巴努商店)`)
	tradeRouteRE  = regexp.MustCompile(`(?i)(跑商航线|贸易航线|推荐航线|整条线|最优路线)`)
	shipShopRE    = regexp.MustCompile(`(?i)(哪里租船|租船|购点|游戏里哪里买船|船店|new\s*deal)`)
	liveRE        = regexp.MustCompile(`(?i)(patch\s*notes?|spectrum|补丁说明|官方论坛)`)
	docsRE        = regexp.MustCompile(`(?i)(lore|设定|背景|wiki\s*摘要|是什么来头)`)
	buyRE         = regexp.MustCompile(`(?i)(买|买入|哪里买|在哪买|\bbuy\b)`)
	sellRE        = regexp.MustCompile(`(?i)(卖|卖出|哪里卖|在哪卖|哪出|\bsell\b)`)
	obtainCtxRE   = regexp.MustCompile(`(?i)(哪里买|在哪买|能买到|买得到|多少钱|获取|怎么得|如何得|掉落|奖励|\bbuy\b|\bobtain\b|买)`)
	// RE2 has no lookbehind; filter token boundaries in extract helpers.
	shortcodeRE    = regexp.MustCompile(`(?i)([A-Za-z]{1,8}\d+[A-Za-z0-9\-]*|[A-Za-z]+\-\d+[A-Za-z0-9]*)`)
	nameTokenRE    = regexp.MustCompile(`(?i)([A-Za-z]{4,16})`)
	blueprintCtxRE = regexp.MustCompile(`(?i)(蓝图|制造|制作|配方|材料|获取|crafting|blueprint)`)
)

var nameTokenStop = map[string]struct{}{
	"blueprint": {}, "blueprints": {}, "crafting": {}, "craft": {}, "materials": {}, "material": {},
	"where": {}, "what": {}, "when": {}, "with": {}, "from": {}, "this": {}, "that": {}, "have": {},
	"need": {}, "make": {}, "made": {}, "obtain": {}, "price": {}, "prices": {}, "buy": {}, "sell": {},
	"loadout": {}, "rifle": {}, "pistol": {}, "energy": {}, "assault": {}, "weapon": {}, "weapons": {},
}

var cjkSingleAllow = map[rune]struct{}{
	'铝': {}, '铁': {}, '钨': {}, '金': {}, '铜': {}, '钛': {}, '钢': {}, '锡': {},
	'铅': {}, '锌': {}, '钴': {}, '镍': {}, '镁': {}, '锂': {}, '铍': {}, '银': {},
}

// QueryPlan classifies a question into MCP/browser steps (parity with Python query_plan).
func QueryPlan(db *sql.DB, question, hintEntity string) (map[string]any, error) {
	q := strings.TrimSpace(question)
	hint := strings.TrimSpace(hintEntity)
	entities, err := resolveEntities(db, hint, q)
	if err != nil {
		return nil, err
	}
	resolveText := mcpResolveText(hint, entities)
	searchTerm := browserSearchTerm(entities)
	primary := ""
	if len(entities) > 0 {
		primary = str(entities[0]["canonical_name"])
	}

	if miningRE.MatchString(q) {
		conf := "medium"
		if searchTerm != "" {
			conf = "high"
		}
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		return map[string]any{
			"intent": "mining_calc", "path": "mcp_then_browser", "confidence": conf,
			"entities": entities, "mcp_steps": steps, "browser": browserMining(searchTerm),
			"stop_if": "只要矿船静态参数时不开采矿站", "reason": "采矿配装/模拟需交互站",
		}, nil
	}

	if loadoutRE.MatchString(q) {
		conf := "medium"
		if searchTerm != "" {
			conf = "high"
		}
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		if primary != "" {
			steps = append(steps, map[string]any{"tool": "sc_entity_facts", "args": map[string]any{"name_or_id": primary}})
		}
		return map[string]any{
			"intent": "loadout_calc", "path": "mcp_then_browser", "confidence": conf,
			"entities": entities, "mcp_steps": steps, "browser": browserErkul(searchTerm),
			"stop_if": "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
			"reason":  "改装先社区主流成文；确认后再开 Erkul 计算器",
		}, nil
	}

	if missionListRE.MatchString(q) {
		if faction := factionPrimary(entities); faction != nil {
			fname := str(faction["canonical_name"])
			ents := []map[string]any{faction}
			for _, e := range entities {
				if asInt64(e["id"]) != asInt64(faction["id"]) {
					ents = append(ents, e)
				}
			}
			steps := []map[string]any{}
			if resolveText != "" {
				steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
			}
			steps = append(steps, map[string]any{"tool": "sc_list_missions", "args": map[string]any{"faction": fname, "limit": 8}})
			return map[string]any{
				"intent": "mission_list", "path": "mcp_only", "confidence": "high",
				"entities": ents, "mcp_steps": steps, "browser": browserOff(),
				"stop_if": "任务列表已够则不要开浏览器", "reason": "势力任务列表走本地 SCMDB",
			}, nil
		}
	}

	if pricesRE.MatchString(q) && commodityOrTerminalPrimary(entities) {
		commodity := primary
		if commodity == "" {
			commodity = resolveText
		}
		side := priceSide(q)
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		steps = append(steps, map[string]any{"tool": "sc_best_prices", "args": map[string]any{"commodity": commodity, "side": side}})
		return map[string]any{
			"intent": "prices", "path": "mcp_only", "confidence": "high",
			"entities": entities, "mcp_steps": steps, "browser": browserOff(),
			"stop_if": "有报价则不要开浏览器", "reason": "物价走本地 UEX 快照",
		}, nil
	}

	if clientRE.MatchString(q) {
		return guidePlan("client_help", "client", entities, resolveText, "客户端/报错/汉化需教程外链；本地不编造排障步骤", nil), nil
	}
	if pledgeRE.MatchString(q) {
		return guidePlan("pledge_meta", "pledge", entities, resolveText, "氪金/CCU/退款名词需官方与社区指南；库无账单数据", nil), nil
	}
	if eventRE.MatchString(q) {
		return guidePlan("event_guide", "event", entities, resolveText, "活动本/试飞时效强，给外链供 Agent 推理引用", nil), nil
	}
	if shipShopRE.MatchString(q) {
		steps := []map[string]any{}
		if primary != "" {
			steps = append(steps, map[string]any{"tool": "sc_entity_facts", "args": map[string]any{"name_or_id": primary}})
		}
		return guidePlan("ship_shop", "ship_shop", entities, resolveText, "船店/租借购点以 Everark 等外链为准；本地可附静态参数", steps), nil
	}
	if tradeRouteRE.MatchString(q) {
		return guidePlan("trade_route", "trade_route", entities, resolveText, "整条跑商航线需规划站；可辅以本地单价", nil), nil
	}
	if earnMetaRE.MatchString(q) && factionPrimary(entities) == nil {
		return guidePlan("earn_meta", "earn_meta", entities, resolveText, "赚钱开荒需推理组织玩法；附任务/Wiki 外链，不编造时薪",
			[]map[string]any{{"tool": "sc_list_missions", "args": map[string]any{"limit": 5}}}), nil
	}
	if howtoRE.MatchString(q) {
		return guidePlan("howto", "howto", entities, resolveText, "操作/找路教程类；本地证据+外链供 LLM 成文", nil), nil
	}

	if liveRE.MatchString(q) {
		return map[string]any{
			"intent": "live_web", "path": "browser_only", "confidence": "high",
			"entities": entities, "mcp_steps": []map[string]any{},
			"browser": map[string]any{
				"needed": true, "site": "rsi",
				"start_url": "https://robertsspaceindustries.com",
				"search_term": q, "max_actions": 8,
				"extract": []any{"patch version", "relevant notes"},
				"do_not":  []any{"open /auth", "/settings"},
			},
			"stop_if": "", "reason": "Patch/Spectrum 需官网或论坛",
		}, nil
	}

	if docsRE.MatchString(q) {
		return map[string]any{
			"intent": "docs", "path": "mcp_only", "confidence": "medium",
			"entities": entities,
			"mcp_steps": []map[string]any{{"tool": "sc_search_docs", "args": map[string]any{"query": q, "limit": 5}}},
			"browser": browserOff(), "stop_if": "库有摘要则不要开浏览器", "reason": "本地 Wiki FTS",
		}, nil
	}

	if len(entities) > 0 {
		name := str(entities[0]["canonical_name"])
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		steps = append(steps, map[string]any{"tool": "sc_entity_facts", "args": map[string]any{"name_or_id": name}})
		return map[string]any{
			"intent": "facts", "path": "mcp_only", "confidence": "high",
			"entities": entities, "mcp_steps": steps, "browser": browserOff(),
			"stop_if": "facts 已够答则不要开浏览器", "reason": "静态实体事实走本地库",
		}, nil
	}

	steps := []map[string]any{}
	if resolveText != "" {
		steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
	}
	return map[string]any{
		"intent": "facts", "path": "mcp_only", "confidence": "low",
		"entities": []map[string]any{}, "mcp_steps": steps, "browser": browserOff(),
		"stop_if": "未解析到实体前不要盲目开浏览器", "reason": "无法判断意图；先 resolve",
	}, nil
}

func mcpResolveText(hint string, entities []map[string]any) string {
	if hint != "" {
		return hint
	}
	if len(entities) > 0 {
		e := entities[0]
		if zh := str(e["zh"]); zh != "" {
			return zh
		}
		return str(e["canonical_name"])
	}
	return ""
}

func browserSearchTerm(entities []map[string]any) string {
	if len(entities) > 0 {
		return str(entities[0]["canonical_name"])
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

func entityBrief(m map[string]any) map[string]any {
	brief := map[string]any{
		"id": m["id"], "kind": m["kind"], "canonical_name": m["canonical_name"],
	}
	if str(m["matched_lang"]) == "zh" {
		if alias := str(m["matched_alias"]); alias != "" {
			brief["zh"] = alias
		}
	}
	return brief
}

func aliasLenOK(alias string) bool {
	a := strings.TrimSpace(alias)
	if a == "" {
		return false
	}
	if isASCII(a) {
		return len(a) >= 3
	}
	runes := []rune(a)
	if len(runes) == 1 {
		_, ok := cjkSingleAllow[runes[0]]
		return ok
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func resolveEntities(db *sql.DB, hint, question string) ([]map[string]any, error) {
	seen := map[int64]bool{}
	out := []map[string]any{}
	add := func(m map[string]any) {
		eid := asInt64(m["id"])
		if seen[eid] {
			return
		}
		seen[eid] = true
		out = append(out, entityBrief(m))
	}
	for _, t := range []string{hint, question} {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		hits, err := ResolveName(db, t)
		if err != nil {
			return nil, err
		}
		for _, m := range hits {
			add(m)
		}
	}
	q := strings.TrimSpace(question)
	if q != "" {
		preferBP := wantsBlueprintPrefer(q)
		tokens := extractShortcodes(q)
		if preferBP {
			tokens = append(tokens, extractNameTokens(q)...)
		}
		for _, tok := range tokens {
			hits, err := ResolveName(db, tok)
			if err != nil {
				return nil, err
			}
			if preferBP {
				seenHit := map[int64]bool{}
				for _, m := range hits {
					seenHit[asInt64(m["id"])] = true
				}
				bps, err := resolveBlueprintsForToken(db, tok)
				if err != nil {
					return nil, err
				}
				for _, m := range bps {
					id := asInt64(m["id"])
					if !seenHit[id] {
						hits = append(hits, m)
						seenHit[id] = true
					}
				}
			}
			for _, m := range hits {
				add(m)
			}
		}
		embedded, err := entitiesEmbeddedInQuestion(db, q)
		if err != nil {
			return nil, err
		}
		for _, m := range embedded {
			add(m)
		}
	}
	return preferBlueprintPrimary(out, q), nil
}

func entitiesEmbeddedInQuestion(db *sql.DB, question string) ([]map[string]any, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       a.alias AS matched_alias, a.lang AS matched_lang
		FROM aliases a
		JOIN entities e ON e.id = a.entity_id
		WHERE (? LIKE '%' || a.alias || '%') COLLATE NOCASE
		  AND length(trim(a.alias)) >= 1
		ORDER BY length(a.alias) DESC
		LIMIT 25
	`, q)
	if err != nil {
		return nil, err
	}
	items, err := scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	filtered := []map[string]any{}
	for _, r := range items {
		if aliasLenOK(str(r["matched_alias"])) {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		rows2, err := db.Query(`
			SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
			       e.canonical_name AS matched_alias, 'en' AS matched_lang
			FROM entities e
			WHERE (? LIKE '%' || e.canonical_name || '%') COLLATE NOCASE
			  AND length(trim(e.canonical_name)) >= 2
			ORDER BY length(e.canonical_name) DESC
			LIMIT 25
		`, q)
		if err != nil {
			return nil, err
		}
		items, err = scanMaps(rows2)
		rows2.Close()
		if err != nil {
			return nil, err
		}
		for _, r := range items {
			if aliasLenOK(str(r["matched_alias"])) {
				filtered = append(filtered, r)
			}
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	ranked := rankRows(filtered)
	rankOf := func(kind string) int {
		if r, ok := kindRank[kind]; ok {
			return r
		}
		return 99
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		li := len(str(ranked[i]["matched_alias"]))
		lj := len(str(ranked[j]["matched_alias"]))
		if li != lj {
			return li > lj
		}
		ki := rankOf(str(ranked[i]["kind"]))
		kj := rankOf(str(ranked[j]["kind"]))
		if ki != kj {
			return ki < kj
		}
		return str(ranked[i]["canonical_name"]) < str(ranked[j]["canonical_name"])
	})
	return ranked, nil
}

func extractShortcodes(question string) []string {
	found := []string{}
	seen := map[string]bool{}
	for _, loc := range shortcodeRE.FindAllStringSubmatchIndex(question, -1) {
		if len(loc) < 4 {
			continue
		}
		start, end := loc[2], loc[3]
		if !tokenBoundaryOK(question, start, end) {
			continue
		}
		tok := question[start:end]
		key := strings.ToLower(tok)
		if seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, tok)
	}
	return found
}

func extractNameTokens(question string) []string {
	found := []string{}
	seen := map[string]bool{}
	for _, loc := range nameTokenRE.FindAllStringSubmatchIndex(question, -1) {
		if len(loc) < 4 {
			continue
		}
		start, end := loc[2], loc[3]
		if !tokenBoundaryOK(question, start, end) {
			continue
		}
		tok := question[start:end]
		key := strings.ToLower(tok)
		if _, stop := nameTokenStop[key]; stop || seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, tok)
	}
	return found
}

func tokenBoundaryOK(s string, start, end int) bool {
	if start > 0 {
		prev := s[start-1]
		if (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9') {
			return false
		}
	}
	if end < len(s) {
		next := s[end]
		if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') {
			return false
		}
	}
	return true
}

func resolveBlueprintsForToken(db *sql.DB, token string) ([]map[string]any, error) {
	tok := strings.TrimSpace(token)
	if len(tok) < 2 {
		return nil, nil
	}
	like := "%" + tok + "%"
	rows, err := db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       COALESCE(a.alias, e.canonical_name) AS matched_alias,
		       COALESCE(a.lang, 'en') AS matched_lang
		FROM entities e
		LEFT JOIN aliases a ON a.entity_id = e.id
		WHERE e.kind = 'blueprint'
		  AND (
		    e.canonical_name LIKE ? COLLATE NOCASE
		    OR a.alias LIKE ? COLLATE NOCASE
		  )
		ORDER BY length(e.canonical_name) ASC
		LIMIT 25
	`, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}

func wantsBlueprintPrefer(q string) bool {
	return blueprintCtxRE.MatchString(q) || obtainCtxRE.MatchString(q)
}

func preferBlueprintPrimary(entities []map[string]any, question string) []map[string]any {
	if len(entities) == 0 || !wantsBlueprintPrefer(question) {
		return entities
	}
	bps := []map[string]any{}
	rest := []map[string]any{}
	for _, e := range entities {
		if str(e["kind"]) == "blueprint" {
			bps = append(bps, e)
		} else {
			rest = append(rest, e)
		}
	}
	if len(bps) == 0 {
		return entities
	}
	tokens := append(extractShortcodes(question), extractNameTokens(question)...)
	sort.SliceStable(bps, func(i, j int) bool {
		return blueprintSortLess(bps[i], bps[j], tokens)
	})
	return append(bps, rest...)
}

func blueprintSortLess(a, b map[string]any, tokens []string) bool {
	ka := blueprintSortKey(a, tokens)
	kb := blueprintSortKey(b, tokens)
	if ka.skin != kb.skin {
		return ka.skin < kb.skin
	}
	if ka.tokenRank != kb.tokenRank {
		return ka.tokenRank < kb.tokenRank
	}
	if ka.nameLen != kb.nameLen {
		return ka.nameLen < kb.nameLen
	}
	return ka.lower < kb.lower
}

type bpKey struct {
	skin, tokenRank, nameLen int
	lower                    string
}

func blueprintSortKey(entity map[string]any, tokens []string) bpKey {
	name := str(entity["canonical_name"])
	lower := strings.ToLower(name)
	skin := 0
	if strings.Contains(name, `"`) || strings.Contains(name, "“") || strings.Contains(name, "”") {
		skin = 1
	}
	tokenRank := 99
	for i, tok := range tokens {
		t := strings.ToLower(tok)
		if lower == t || strings.HasPrefix(lower, t+" ") || strings.HasPrefix(lower, t+"-") {
			if i < tokenRank {
				tokenRank = i
			}
		}
	}
	return bpKey{skin: skin, tokenRank: tokenRank, nameLen: len(name), lower: lower}
}

func commodityOrTerminalPrimary(entities []map[string]any) bool {
	if len(entities) == 0 {
		return true
	}
	k := str(entities[0]["kind"])
	return k == "commodity" || k == "terminal"
}

func factionPrimary(entities []map[string]any) map[string]any {
	for _, e := range entities {
		if str(e["kind"]) == "faction" {
			return e
		}
	}
	return nil
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
		"extract": []any{"community recommended loadout", "selected ship", "DPS summary", "shield / power notes"},
		"do_not": []any{"open /auth", "/settings", "wander other sites first", "open calculator before community answer unless user confirmed"},
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
		"extract": []any{"laser / module setup", "instability / resistance notes"},
		"do_not": []any{"open /auth", "/settings", "wander other sites first"},
		"wait_confirm": true, "leave_open": true, "handoff_to_user": true,
	}
}

func guidePlan(intent, category string, entities []map[string]any, resolveText, reason string, mcpSteps []map[string]any) map[string]any {
	steps := append([]map[string]any{}, mcpSteps...)
	hasResolve := false
	for _, s := range steps {
		if str(s["tool"]) == "sc_resolve_name" {
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
		"reason": reason, "guide_category": category,
	}
}
