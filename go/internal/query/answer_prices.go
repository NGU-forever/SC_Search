package query

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerPrices(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	side := "sell"
	if steps, ok := c.Plan["mcp_steps"].([]map[string]any); ok {
		for _, step := range steps {
			if anyx.String(step["tool"]) == "sc_best_prices" {
				if args, ok := step["args"].(map[string]any); ok {
					if s := anyx.String(args["side"]); s != "" {
						side = s
					}
				}
			}
		}
	} else if stepsAny, ok := c.Plan["mcp_steps"].([]any); ok {
		for _, s := range stepsAny {
			if step, ok := s.(map[string]any); ok && anyx.String(step["tool"]) == "sc_best_prices" {
				if args, ok := step["args"].(map[string]any); ok {
					if ss := anyx.String(args["side"]); ss != "" {
						side = ss
					}
				}
			}
		}
	}
	terminalName := terminalFromContext(c.Entities, c.Question)
	commodityEntity := firstKind(c.Entities, "commodity")
	commodity := c.Primary
	if commodityEntity != nil {
		commodity = anyx.String(commodityEntity["canonical_name"])
	} else if commodity == "" {
		commodity = c.Question
	}
	var rows []map[string]any
	pricesRemain := false
	if terminalName != "" {
		all, err := TerminalPrices(c.DB, terminalName, side)
		if err != nil {
			return res, err
		}
		if commodityEntity != nil {
			cn := strings.ToLower(anyx.String(commodityEntity["canonical_name"]))
			filtered := []map[string]any{}
			for _, r := range all {
				if strings.ToLower(anyx.String(r["commodity"])) == cn {
					filtered = append(filtered, r)
				}
			}
			all = filtered
		}
		pricesRemain = len(all) > c.PriceLimit
		if len(all) > c.PriceLimit {
			rows = all[:c.PriceLimit]
		} else {
			rows = all
		}
	} else {
		fetched, err := BestPrices(c.DB, commodity, side, "", c.PriceLimit+1)
		if err != nil {
			return res, err
		}
		pricesRemain = len(fetched) > c.PriceLimit
		if len(fetched) > c.PriceLimit {
			rows = fetched[:c.PriceLimit]
		} else {
			rows = fetched
		}
	}
	ev["prices"] = slimPrices(rows)
	if len(rows) > 0 {
		ev["reasoning_hint"] = deepOrganizeHint
		top := rows[0]
		verSuffix := draftLiveSuffix(c.Live, anyx.String(top["game_version"]))
		stale := IsStaleVsLive(c.Live, anyx.String(top["game_version"]))
		best := "较优"
		if stale {
			// Stale snapshot: never present as the current Live best price.
			verSuffix = priceStaleSuffix(c.Live, anyx.String(top["game_version"]))
			best = "快照中较优"
			ev["meta"] = map[string]any{
				"stale":         true,
				"stale_sources": []string{anyx.String(top["source"])},
				"game_version":  anyx.String(top["game_version"]),
				"as_of":         c.Live,
			}
		}
		core := ""
		if terminalName != "" && commodityEntity == nil {
			core = fmt.Sprintf("%s %s 报价示例：%v 约 %v aUEC%s。", terminalName, side, top["commodity"], top["price"], verSuffix)
		} else {
			core = fmt.Sprintf("%v %s %s：%v 约 %v aUEC%s。", top["commodity"], side, best, top["terminal"], top["price"], verSuffix)
		}
		res.Draft = "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + core
		res.Detail = pricesRemain
		if pricesRemain {
			if side == "sell" {
				res.FollowUps = []string{"看更多终端", "查买入价"}
			} else {
				res.FollowUps = []string{"看更多终端", "查卖出价"}
			}
		}
	} else {
		res.Missing = append(res.Missing, "prices")
		target := terminalName
		if target == "" {
			target = commodity
		}
		res.Draft = fmt.Sprintf("本地没有 %s 的 %s 报价。", target, side)
	}
	return res, nil
}

func terminalFromContext(entities []map[string]any, question string) string {
	if t := firstKind(entities, "terminal"); t != nil {
		return anyx.String(t["canonical_name"])
	}
	_ = question
	return ""
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

// priceStaleSuffix is the mandatory wording when a price snapshot lags the Live version.
func priceStaleSuffix(live, sourceVer string) string {
	return fmt.Sprintf("（⚠ 价格快照版本 %s，落后 Live %s，非当前 Live 报价，仅供参考；可说「同步数据」刷新）",
		ShortLiveLabel(sourceVer), live)
}
