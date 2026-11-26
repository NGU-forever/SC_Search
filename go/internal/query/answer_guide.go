package query

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

const openGuideConfirm = "要用浏览器打开推荐教程吗？"

var guideReasoning = map[string]string{
	"howto":       "操作/找路类：用外链与常识按步骤讲清，勿编造当前补丁独有 UI 细节。",
	"client_help": "客户端/报错：先列常见排查方向（USER/Reset/换区），具体错误码以教程为准。",
	"earn_meta":   "赚钱开荒：按玩法类型组织建议；可用任务样例推理，勿编造时薪/最优线。",
	"pledge_meta": "氪金名词：解释 CCU/WB/信用点/退款概念，金额与活动以官网为准。",
	"event_guide": "活动本：说明玩法框架与风险，逐步流程引用外链，勿编造刷新点。",
	"trade_route": "跑商航线：可结合本地单价推理；整条最优线指向规划站。",
	"ship_shop":   "船店/租借：本地给静态参数；购点/租金指向 Everark 等外链。",
}

func answerGuideIntent(c *answerContext) (map[string]any, error) {
	ev := c.evidence()
	browser, _ := c.Plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{}
	}
	resources := castStringMaps(browser["resources"])
	ev["guides"] = resources
	ev["reasoning_hint"] = guideReasoning[c.Intent]
	missionBits := []string{}
	staticBit := ""
	missing := []string{}

	if c.Intent == "earn_meta" {
		list, err := ListMissions(c.DB, "", "", "", 5)
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
		ev["missions"] = missions
		for i, r := range list.Rows {
			if i >= 4 {
				break
			}
			title := anyx.String(r["title"])
			if title == "" {
				title = anyx.String(r["canonical_name"])
			}
			bit := title
			if mt := anyx.String(r["mission_type"]); mt != "" {
				bit += "[" + mt + "]"
			}
			if r["reward_uec"] != nil {
				bit += fmt.Sprintf("~%daUEC", asInt64(r["reward_uec"]))
			}
			missionBits = append(missionBits, bit)
		}
	}
	if c.Intent == "ship_shop" && c.Primary != "" {
		bundle, err := EntityFacts(c.DB, c.Primary, "full")
		if err != nil {
			return nil, err
		}
		if bundle != nil {
			facts := anyx.Maps(bundle["facts"])
			for _, f := range facts {
				key := anyx.String(f["key"])
				if key == "cargo_scu" || key == "cargo_capacity" {
					staticBit = "静态参考：货舱 " + factValue(slimFact(f)) + "。"
					ev["facts"] = []map[string]any{slimFact(f)}
					break
				}
			}
		}
	}
	if len(resources) == 0 {
		missing = append(missing, "guides")
	}
	draft := draftGuide(c.Intent, orLabel(c.Label, c.Primary), c.Live, resources, missionBits, staticBit)
	ev["missing"] = missing
	return answerOut(c.Intent, c.Path, "medium", c.Live, draft, ev, len(resources) > 0 || len(missionBits) > 0, []string{openGuideConfirm}, browser, c.Plan), nil
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
