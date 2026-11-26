package query

import (
	"fmt"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerCalcIntent(c *answerContext) (map[string]any, error) {
	ev := c.evidence()
	followUps := []string{}
	detailAvailable := false
	draft := ""
	missing := []string{}
	hadScored := false

	if c.Primary == "" {
		draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
		missing = append(missing, "entity")
	} else {
		bundle, err := EntityFacts(c.DB, c.Primary, "full")
		if err != nil {
			return nil, err
		}
		if bundle == nil {
			missing = append(missing, "entity_facts")
			draft = fmt.Sprintf("未找到 %s 的本地事实。", orLabel(c.Label, c.Primary))
		} else {
			allFacts := anyx.Maps(bundle["facts"])
			picked, scored := pickFacts(allFacts, c.Question, c.FactLimit)
			hadScored = scored
			ev["facts"] = picked
			site := "Erkul"
			if c.Intent == "mining_calc" {
				site = "采矿工具站"
			}
			static := ""
			if len(picked) > 0 {
				static = fmt.Sprintf("静态参考：%v=%s。", picked[0]["key"], factValue(picked[0]))
			}
			browser, _ := c.Plan["browser"].(map[string]any)
			guideLinks := formatGuideLinks(castStringMaps(browser["resources"]), 3)
			if c.Intent == "loadout_calc" {
				draft = fmt.Sprintf("%s 改装：先给 Erkul 社区推荐或出厂主流方案（%s本地静态可作对照）。不要一上来开计算器；成文后再问是否打开 Erkul Browse/计算器；用户确认后打开并 leave_open。", orLabel(c.Label, c.Primary), static)
				followUps = []string{"按社区主流推荐改装", "打开 Erkul 自己改", "只要静态参数", openGuideConfirm}
			} else {
				draft = fmt.Sprintf("%s 配装/计算需 %s。%s本地只能给静态参数；Agent 可先据静态参数推理建议，要精确破矿模拟再说，确认后打开工具站；打开并摘要后请把浏览器留在会话里（leave_open），方便用户自己继续改。", orLabel(c.Label, c.Primary), site, static)
				followUps = []string{"打开 " + site + " 查配装", "只要静态参数", openGuideConfirm}
			}
			if guideLinks != "" {
				draft += "\n参考：\n" + guideLinks
			}
			detailAvailable = true
			_ = hadScored
		}
	}
	ev["missing"] = missing
	browser, _ := c.Plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{"needed": false}
	}
	conf := recomputeConfidence(c.Intent, anyx.String(c.Plan["confidence"]), hadScored, len(c.Entities) > 0, anyx.Maps(ev["facts"]), nil, nil, missing)
	if conf == "" {
		conf = "medium"
	}
	if c.Intent == "loadout_calc" || c.Intent == "mining_calc" {
		if len(anyx.Maps(ev["facts"])) > 0 {
			conf = "medium"
		} else {
			conf = "low"
		}
	}
	return answerOut(c.Intent, c.Path, conf, c.Live, draft, ev, detailAvailable, followUps, browser, c.Plan), nil
}
