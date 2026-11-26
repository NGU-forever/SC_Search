package query

import (
	"database/sql"
	"regexp"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var (
	// loadout: require explicit refit/loadout wording; bare「火力」 alone is ship_stats (e.g. 火力多大).
	loadoutRE = regexp.MustCompile(`(?i)(dps|loadout|erkul|改装|配装|怎么配|如何配|打炮配|护盾续航)`)
	// mining: combo of mining + laser/module, or explicit laser/head terms — not bare「采矿船货舱」.
	miningCoreRE   = regexp.MustCompile(`(?i)(采矿|挖矿|\bmining\b)`)
	miningSetupRE  = regexp.MustCompile(`(?i)(激光|模块|配装|配激光|破矿|阻力|精炼模拟|regolith|激光头|采矿头|mining\s*laser)`)
	miningStaticRE = regexp.MustCompile(`(?i)(货舱|舱容|参数|多大|多重|质量|厂家|制造商)`)
	pricesRE       = regexp.MustCompile(`(?i)(价格|报价|什么价|售价|收购价|最低价|最高价|最便宜|最贵|哪里买|哪里卖|在哪买|在哪卖|哪[买卖出]|买点|卖点|买入|卖出|多少钱|\bbuy\b|\bsell\b|price)`)
	missionListRE  = regexp.MustCompile(`(?i)(有哪些任务|哪些任务|任务列表|什么任务|刷什么任务|任务有哪些|能接什么任务|刷声望|声望怎么刷|声望任务)`)
	missionMoneyRE = regexp.MustCompile(`(?i)(手续费|买进|buy\s*in|冷却|cooldown|弃任|abandoned|站点池|site_?pool|有哪些站点|刷法|任务机制|敌船|敌舰|lifetime|时效|要交多少|交多少钱|任务.*多少钱|多少钱.*任务|报酬|奖励|收益|\breward\b)`)
	missionRepRE   = regexp.MustCompile(`(?i)(junior|声望|等级|能接什么任务|声望要求|刷声望|声望怎么刷)`)
	clientRE       = regexp.MustCompile(`(?i)(报错|进不去|打不开|锁服|汉化|虚拟内存|鼠标失灵|19000|32212|删\s*user|角色重置|reset\b|ptu怎么进|eptu|启动器)`)
	howtoRE        = regexp.MustCompile(`(?i)(怎么走|如何走|叫船|机库|床上?登出|按键|键位|跃迁怎么|量子怎么|怎么起飞|怎么降落|对接|找路|路线|操作教程|新手教程|怎么上(我的)?飞?船|复活点|出生点)`)
	earnMetaRE     = regexp.MustCompile(`(?i)(怎么赚钱|如何赚钱|怎么刷钱|开荒|萌新(该|要)?干什么|新手(该|要)?干什么|第一桶金|赚钱方法|推荐玩法|怎么玩)`)
	pledgeRE       = regexp.MustCompile(`(?i)(ccu|warbond|\bwb\b|融船|信用点|store\s*credit|叠叠乐|退款|灰市|star\s*hangar|资格包|新手包|邀请码|lti)`)
	eventRE        = regexp.MustCompile(`(?i)(invictus|无敌舰队|890\s*登舰|奥里森之围|海盗潮|iae|free\s*fly|免费试飞|活动怎么|北极星兑换|巴努商店)`)
	tradeRouteRE   = regexp.MustCompile(`(?i)(跑商航线|贸易航线|推荐航线|整条线|最优路线)`)
	shipShopRE     = regexp.MustCompile(`(?i)(哪里租船|租船|购点|游戏里哪里买船|买船|船店|new\s*deal)`)
	shipBuyAskRE   = regexp.MustCompile(`(?i)(哪里买|在哪买|能买到|买得到|\bbuy\b)`)
	liveRE         = regexp.MustCompile(`(?i)(patch\s*notes?|spectrum|补丁说明|官方论坛)`)
	liveLocalRE    = regexp.MustCompile(`(?i)(当前\s*live|live\s*版本|游戏版本|现在(是|的)?版本|最新版本)`)
	dataSyncRE     = regexp.MustCompile(`(?i)(本地数据|数据.*同步|什么时候同步|何时同步|同步时间|ingest)`)
	docsRE         = regexp.MustCompile(`(?i)(lore|设定|背景|wiki\s*摘要|是什么来头)`)
	buyRE          = regexp.MustCompile(`(?i)(买|买入|哪里买|在哪买|\bbuy\b)`)
	sellRE         = regexp.MustCompile(`(?i)(卖|卖出|哪里卖|在哪卖|哪出|\bsell\b)`)
	obtainCtxRE    = regexp.MustCompile(`(?i)(哪里买|在哪买|能买到|买得到|多少钱|获取|怎么得|如何得|掉落|奖励|\bbuy\b|\bobtain\b|买)`)
	// RE2 has no lookbehind; filter token boundaries in extract helpers.
	shortcodeRE    = regexp.MustCompile(`(?i)([A-Za-z]{1,8}\d+[A-Za-z0-9\-]*|[A-Za-z]+\-\d+[A-Za-z0-9]*)`)
	nameTokenRE    = regexp.MustCompile(`(?i)([A-Za-z]{4,16})`)
	blueprintCtxRE = regexp.MustCompile(`(?i)(蓝图|制造|制作|配方|材料|获取|crafting|blueprint)`)
)

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
		primary = anyx.String(entities[0]["canonical_name"])
	}

	// Action priority comes from DetectAction / actionRules (not if-order alone).
	act := DetectAction(q)

	if act.Action == ActionMining {
		ents := filterMiningEntities(entities, q)
		searchTerm = browserSearchTerm(ents)
		conf := "medium"
		if searchTerm != "" {
			conf = "high"
		}
		steps := []map[string]any{}
		rt := mcpResolveText(hint, ents)
		if rt != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": rt}})
		}
		return map[string]any{
			"intent": "mining_calc", "path": "mcp_then_browser", "confidence": conf,
			"entities": ents, "mcp_steps": steps, "browser": browserMining(searchTerm),
			"stop_if": "只要矿船静态参数时不开采矿站", "reason": "采矿配装/模拟需交互站",
			"action_rule": act.RuleID,
		}, nil
	}

	if act.Action == ActionLoadout {
		conf := "medium"
		if searchTerm != "" {
			conf = "high"
		}
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		if primary != "" && entityKind(entities, 0) == "ship" {
			steps = append(steps, map[string]any{"tool": "sc_entity_facts", "args": map[string]any{"name_or_id": primary}})
		}
		return map[string]any{
			"intent": "loadout_calc", "path": "mcp_then_browser", "confidence": conf,
			"entities": entities, "mcp_steps": steps, "browser": browserErkul(searchTerm),
			"stop_if":     "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
			"reason":      "改装先社区主流成文；确认后再开 Erkul 计算器",
			"action_rule": act.RuleID,
		}, nil
	}

	if act.Action == ActionMissionMoney {
		steps := []map[string]any{}
		if resolveText != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": resolveText}})
		}
		if faction := factionPrimary(entities); faction != nil {
			steps = append(steps, map[string]any{"tool": "sc_list_missions", "args": map[string]any{"faction": anyx.String(faction["canonical_name"]), "limit": 8}})
		}
		if primary != "" {
			steps = append(steps, map[string]any{"tool": "sc_entity_facts", "args": map[string]any{"name_or_id": primary}})
		}
		return map[string]any{
			"intent": "mission_money", "path": "mcp_only", "confidence": "high",
			"entities": entities, "mcp_steps": steps, "browser": browserOff(),
			"stop_if": "机制字段已够则不要开浏览器", "reason": "任务手续费/冷却/站点走本地 SCMDB",
			"action_rule": act.RuleID,
		}, nil
	}

	if act.Action == ActionMissionList {
		if faction := factionPrimary(entities); faction != nil {
			fname := anyx.String(faction["canonical_name"])
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
				"action_rule": act.RuleID,
			}, nil
		}
	}

	if pricesRE.MatchString(q) && hasCommodityOrTerminal(entities) {
		ents := preferCommodityEntities(entities)
		commodity := ""
		if len(ents) > 0 {
			commodity = anyx.String(ents[0]["canonical_name"])
		}
		if commodity == "" {
			commodity = resolveText
		}
		side := priceSide(q)
		steps := []map[string]any{}
		rt := mcpResolveText(hint, ents)
		if rt == "" {
			rt = commodity
		}
		if rt != "" {
			steps = append(steps, map[string]any{"tool": "sc_resolve_name", "args": map[string]any{"text": rt}})
		}
		steps = append(steps, map[string]any{"tool": "sc_best_prices", "args": map[string]any{"commodity": commodity, "side": side}})
		return map[string]any{
			"intent": "prices", "path": "mcp_only", "confidence": "high",
			"entities": ents, "mcp_steps": steps, "browser": browserOff(),
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
	if shipShopRE.MatchString(q) || isShipBuyQuestion(q, entities) {
		steps := []map[string]any{}
		if primary != "" && entityKind(entities, 0) == "ship" {
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

	if liveLocalRE.MatchString(q) {
		return map[string]any{
			"intent": "live_local", "path": "mcp_only", "confidence": "high",
			"entities": entities, "mcp_steps": []map[string]any{
				{"tool": "sc_data_freshness", "args": map[string]any{}},
			},
			"browser": browserOff(), "stop_if": "本地 Live 对照已够则不要开浏览器",
			"reason": "当前 Live 版本走本地 freshness / scmdb 对照",
		}, nil
	}
	if dataSyncRE.MatchString(q) {
		return map[string]any{
			"intent": "data_sync", "path": "mcp_only", "confidence": "high",
			"entities": entities, "mcp_steps": []map[string]any{
				{"tool": "sc_data_freshness", "args": map[string]any{}},
			},
			"browser": browserOff(), "stop_if": "同步时间已够则不要开浏览器",
			"reason": "本地 ingest 同步时间",
		}, nil
	}

	if liveRE.MatchString(q) {
		return map[string]any{
			"intent": "live_web", "path": "browser_only", "confidence": "high",
			"entities": entities, "mcp_steps": []map[string]any{},
			"browser": map[string]any{
				"needed": true, "site": "rsi",
				"start_url":   "https://robertsspaceindustries.com",
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
			"entities":  entities,
			"mcp_steps": []map[string]any{{"tool": "sc_search_docs", "args": map[string]any{"query": q, "limit": 5}}},
			"browser":   browserOff(), "stop_if": "库有摘要则不要开浏览器", "reason": "本地 Wiki FTS",
		}, nil
	}

	if len(entities) > 0 {
		name := anyx.String(entities[0]["canonical_name"])
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

// MissionRepContext reports reputation/rank wording for primary selection.
func MissionRepContext(question string) bool {
	return missionRepRE.MatchString(question)
}

// MissionMoneyContext reports fee/cooldown/site mechanism wording.
func MissionMoneyContext(question string) bool {
	return missionMoneyRE.MatchString(question)
}
