package query

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerShopObtain(c *answerContext) (map[string]any, error) {
	ev := c.evidence()

	bundle, err := EntityFacts(c.DB, c.Primary, "full")
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	entityRow, _ := bundle["entity"].(map[string]any)
	srcVer := ""
	if entityRow != nil {
		srcVer = anyx.String(entityRow["game_version"])
	}
	buyRows := []map[string]any{}
	if c.PrimaryKind == "ship" {
		seenTerms := map[string]bool{}
		for _, ent := range c.Entities {
			if anyx.String(ent["kind"]) != "ship" || ent["id"] == nil {
				continue
			}
			rows, err := shipBuyPrices(c.DB, asInt64(ent["id"]), c.PriceLimit)
			if err != nil {
				return nil, err
			}
			for _, row := range rows {
				term := anyx.String(row["terminal"])
				if seenTerms[term] {
					continue
				}
				seenTerms[term] = true
				buyRows = append(buyRows, row)
			}
			if len(buyRows) >= c.PriceLimit {
				break
			}
		}
		sort.SliceStable(buyRows, func(i, j int) bool {
			pi, pj := anyx.Float(buyRows[i]["price"]), anyx.Float(buyRows[j]["price"])
			if pi != pj {
				return pi < pj
			}
			return anyx.String(buyRows[i]["terminal"]) < anyx.String(buyRows[j]["terminal"])
		})
		if len(buyRows) > c.PriceLimit {
			buyRows = buyRows[:c.PriceLimit]
		}
	}
	allFacts := anyx.Maps(bundle["facts"])
	facts := []map[string]any{}
	for i, f := range allFacts {
		if i >= c.FactLimit {
			break
		}
		facts = append(facts, slimFact(f))
	}
	ev["facts"] = facts
	if len(buyRows) > 0 {
		shops := slimPrices(buyRows)
		ev["prices"] = shops
		ev["acquisition"] = acquisitionShopYes(shops, "")
		ev["missing"] = []string{}
		ev["reasoning_hint"] = deepOrganizeHint
		return answerOut(c.Intent, c.Path, "high", c.Live, draftShipShop(orLabel(c.Label, c.Primary), buyRows, c.Live), ev, len(facts) > 0, nil, map[string]any{"needed": false}, c.Plan), nil
	}
	ev["acquisition"] = acquisitionUnknown("", nil, nil, []string{"shop_prices"}, "shop_unknown", "本地未收录游戏内商店/aUEC 报价")
	ev["missing"] = []string{"shop_prices"}
	return answerOut(c.Intent, c.Path, "high", c.Live, draftShopUnknown(orLabel(c.Label, c.Primary), c.Live, srcVer), ev, len(facts) > 0, nil, map[string]any{"needed": false}, c.Plan), nil
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
		term := anyx.String(r["terminal"])
		if term == "" {
			term = "?"
		}
		priceS := "?"
		if r["price"] != nil {
			priceS = fmt.Sprintf("%v", r["price"])
			if f := anyx.Float(r["price"]); f == float64(int64(f)) {
				priceS = fmt.Sprintf("%d", int64(f))
				// add commas roughly
				priceS = formatIntComma(int64(f))
			}
		}
		bits = append(bits, fmt.Sprintf("%s 约 %s aUEC", term, priceS))
		if srcVer == "" {
			srcVer = anyx.String(r["game_version"])
		}
	}
	return "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + label + " 游戏内可购：" + strings.Join(bits, "；") + draftLiveSuffix(live, srcVer) + "。"
}
