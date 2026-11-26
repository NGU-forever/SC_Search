package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func answerFacts(c *answerContext) (answerResult, error) {
	ev := c.evidence()
	res := newAnswerResult(ev)
	if c.Primary == "" {
		res.Draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
		res.Missing = append(res.Missing, "entity")
		return res, nil
	}
	bundle, err := EntityFacts(c.DB, c.Primary, "full")
	if err != nil {
		return res, err
	}
	if bundle == nil {
		res.Missing = append(res.Missing, "entity_facts")
		res.Draft = fmt.Sprintf("未找到 %s 的本地事实。", orLabel(c.Label, c.Primary))
		return res, nil
	}
	allFacts := anyx.Maps(bundle["facts"])
	picked, scored := pickFacts(allFacts, c.Question, c.FactLimit)
	res.HadScored = scored
	ev["facts"] = picked
	srcVer := ""
	if ent, ok := bundle["entity"].(map[string]any); ok {
		srcVer = anyx.String(ent["game_version"])
	}
	if scored && len(picked) > 0 {
		ev["reasoning_hint"] = deepOrganizeHint
		top := picked[0]
		keyLabel := anyx.String(top["key_zh"])
		if keyLabel == "" {
			keyLabel = labelFactKey(anyx.String(top["key"]))
		}
		core := fmt.Sprintf("%s 的 %s 为 %s%s。", c.Label, keyLabel, factValue(top), draftLiveSuffix(c.Live, srcVer))
		res.Detail = len(allFacts) > len(picked)
		if res.Detail {
			core += "需要更细的硬点/部件可以说一声。"
			res.FollowUps = []string{"展开全部参数", "只要货舱"}
		}
		res.Draft = "【供 Agent 深度整理】" + deepOrganizeHint + "\n" + core
	} else {
		res.Draft = fmt.Sprintf("本地库有 %s，但未找到匹配字段。", c.Label)
		res.Missing = append(res.Missing, "matching_facts")
	}
	return res, nil
}

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
		unit := anyx.String(f["unit"])
		num := f["value_num"]
		if unit != "" {
			return fmt.Sprintf("%v %s", num, unit)
		}
		return fmt.Sprint(num)
	}
	return anyx.String(f["value_text"])
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
		key := strings.ToLower(anyx.String(f["key"]))
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
		id := anyx.String(f["key"]) + "|" + factValue(f)
		for _, p := range picked {
			if anyx.String(p["key"])+"|"+factValue(p) == id {
				return
			}
		}
		sf := map[string]any{
			"key": f["key"], "key_zh": labelFactKey(anyx.String(f["key"])),
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
