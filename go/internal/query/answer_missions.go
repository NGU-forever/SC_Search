package query

import (
	"fmt"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerMissions(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	faction := factionPrimary(c.Entities)
	fname := c.Primary
	if faction != nil {
		fname = anyx.String(faction["canonical_name"])
	}
	list, err := ListMissions(c.DB, fname, "", "", 8)
	if err != nil {
		return res, err
	}
	missions := []map[string]any{}
	for _, r := range list.Rows {
		missions = append(missions, map[string]any{
			"canonical_name": r["canonical_name"], "title": r["title"],
			"reward_uec": r["reward_uec"], "systems": r["systems"],
			"mission_type": r["mission_type"],
		})
	}
	ev["missions"] = missions
	if len(list.Rows) > 0 {
		bits := []string{}
		for i, r := range list.Rows {
			if i >= 5 {
				break
			}
			title := anyx.String(r["title"])
			if title == "" {
				title = anyx.String(r["canonical_name"])
			}
			extra := []string{}
			if r["reward_uec"] != nil {
				extra = append(extra, fmt.Sprintf("%v aUEC", asInt64(r["reward_uec"])))
			}
			if sys := anyx.String(r["systems"]); sys != "" {
				extra = append(extra, sys)
			}
			bit := title
			if len(extra) > 0 {
				bit += "（" + strings.Join(extra, "，") + "）"
			}
			bits = append(bits, bit)
		}
		res.Draft = fmt.Sprintf("%s 相关任务示例：%s%s。", orLabel(c.Label, fname), strings.Join(bits, "；"), draftLiveSuffix(c.Live, ""))
		res.Detail = len(list.Rows) > 5 || list.Truncated
		if res.Detail {
			res.FollowUps = []string{"看更多任务"}
		}
	} else {
		res.Missing = append(res.Missing, "missions")
		res.Draft = fmt.Sprintf("本地未找到 %s 的任务列表。", orLabel(c.Label, fname))
	}
	ev["missing"] = res.Missing
	return res, nil
}
