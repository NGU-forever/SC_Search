package orchestrate

import (
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

func missionWantedFields(question string) []string {
	q := strings.ToLower(question)
	out := []string{}
	add := func(f string) {
		for _, x := range out {
			if x == f {
				return
			}
		}
		out = append(out, f)
	}
	askReward := strings.Contains(q, "报酬") || strings.Contains(q, "奖励") || strings.Contains(q, "收益") || strings.Contains(q, "reward")
	askFee := strings.Contains(q, "手续费") || strings.Contains(q, "buy") || strings.Contains(q, "cost") ||
		strings.Contains(q, "买进") || strings.Contains(q, "要交多少") || strings.Contains(q, "交多少钱") ||
		(strings.Contains(q, "多少钱") && (strings.Contains(q, "任务") || strings.Contains(q, "mission")) && !askReward)
	if askFee {
		add("cost")
	}
	if strings.Contains(q, "冷却") || strings.Contains(q, "cooldown") {
		add("cooldown")
	}
	if askReward {
		add("reward")
	}
	if strings.Contains(q, "时效") || strings.Contains(q, "lifetime") {
		add("lifetime")
	}
	if strings.Contains(q, "声望") || strings.Contains(q, "等级") || strings.Contains(q, "junior") || strings.Contains(q, "reputation") {
		add("rep")
	}
	if strings.Contains(q, "站点") || strings.Contains(q, "地区") || strings.Contains(q, "site") || strings.Contains(q, "region") {
		add("site")
	}
	if len(out) == 0 {
		if stPrimaryLooksRep(q) {
			return []string{"rep", "reward"}
		}
		return []string{"cost", "cooldown", "reward", "lifetime", "rep"}
	}
	return out
}

func stPrimaryLooksRep(q string) bool {
	return strings.Contains(q, "声望") || strings.Contains(q, "junior") || strings.Contains(q, "刷声望") ||
		strings.Contains(q, "reputation")
}

func missionFieldLabel(field string) string {
	switch field {
	case "cost":
		return "手续费"
	case "cooldown":
		return "冷却"
	case "reward":
		return "奖励"
	case "lifetime":
		return "时效"
	case "rep":
		return "声望"
	case "site":
		return "站点"
	default:
		return ""
	}
}

func missionField(row map[string]any, field string) (label, value string) {
	switch field {
	case "cost":
		v := formatNum(row["cost_uec"])
		if v == "" {
			return "", ""
		}
		return "手续费", v + " aUEC"
	case "cooldown":
		v := formatNum(row["cooldown"])
		if v == "" {
			return "", ""
		}
		return "冷却", v + " 分钟"
	case "reward":
		v := formatNum(row["reward_uec"])
		if v == "" {
			return "", ""
		}
		return "奖励", v + " aUEC"
	case "lifetime":
		v := formatNum(row["lifetime"])
		if v == "" {
			return "", ""
		}
		return "时效", v + " 小时"
	case "rep":
		parts := []string{}
		if rank := anyx.FirstField(row, "reputation_rank"); rank != "" {
			parts = append(parts, rank)
		}
		if min := formatNum(row["reputation_min"]); min != "" {
			parts = append(parts, "最低声望值 "+min)
		}
		if len(parts) == 0 {
			return "", ""
		}
		return "声望", strings.Join(parts, " / ")
	case "site":
		parts := []string{}
		if r := anyx.FirstField(row, "pyro_regions"); r != "" {
			parts = append(parts, "地区 "+r)
		}
		if s := anyx.FirstField(row, "site_pool"); s != "" {
			parts = append(parts, "站点池 "+truncateRunes(s, 80))
		}
		if len(parts) == 0 {
			return "", ""
		}
		return "站点", strings.Join(parts, "；")
	default:
		return "", ""
	}
}

func firstMissionField(rows []map[string]any, field string) string {
	for _, row := range rows {
		_, v := missionField(row, field)
		if v != "" {
			return v
		}
	}
	return ""
}

func prioritizeMissionFacts(facts []FactBlock, wanted []string) []FactBlock {
	priority := map[string]int{
		"势力": 1, "手续费": 2, "冷却": 3, "奖励": 4, "时效": 5, "声望": 6, "站点": 7, "任务": 8, "星系": 9,
	}
	for _, f := range wanted {
		if lab := missionFieldLabel(f); lab != "" {
			if _, ok := priority[lab]; !ok {
				priority[lab] = 10
			}
		}
	}
	out := append([]FactBlock{}, facts...)
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			pi := priority[out[i].Label]
			pj := priority[out[j].Label]
			if pi == 0 {
				pi = 100
			}
			if pj == 0 {
				pj = 100
			}
			if pj < pi {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// missionRangeParts builds the fee/reward range headline fragments in ask order.
func missionRangeParts(feePool, rewardPool []map[string]any, wanted []string) []string {
	rangeParts := []string{}
	if containsStr(wanted, "cost") {
		if h := feeHeadline(feePool); h != "" {
			rangeParts = append(rangeParts, h)
		}
	}
	if containsStr(wanted, "reward") {
		if h := rewardHeadline(rewardPool); h != "" {
			rangeParts = append(rangeParts, h)
		}
	}
	return rangeParts
}

// buildMissionHeadline prefers numeric ranges, then rep wording, then the top title.
func buildMissionHeadline(faction string, matched []map[string]any, wanted, rangeParts []string) string {
	headline := "任务机制"
	if len(rangeParts) > 0 {
		headline = strings.Join(rangeParts, "；")
	} else if containsStr(wanted, "rep") {
		headline = "声望任务: " + faction
	} else if title := cleanMissionTitle(missionTitle(matched[0])); title != "" {
		headline = title
	}
	return headline
}

// buildMissionFacts walks the displayed rows once, producing both the fact
// blocks and the draft body (without the range prefix).
func buildMissionFacts(faction string, display []map[string]any, wanted []string) ([]FactBlock, string) {
	facts := []FactBlock{}
	seenFact := map[string]struct{}{}
	var draft strings.Builder
	facts = append(facts, FactBlock{Label: "势力", Value: faction})

	for i, row := range display {
		title := cleanMissionTitle(missionTitle(row))
		if title == "" {
			continue
		}
		sys := anyx.FirstField(row, "systems")

		lineParts := []string{title}
		if sys != "" {
			lineParts[0] = title + "（" + sys + "）"
		}

		if i == 0 {
			facts = append(facts, FactBlock{Label: "任务", Value: title})
			if sys != "" {
				facts = append(facts, FactBlock{Label: "星系", Value: sys})
			}
		}

		for _, field := range wanted {
			label, value := missionField(row, field)
			if value == "" {
				continue
			}
			key := label + "|" + value
			if _, dup := seenFact[key]; !dup {
				seenFact[key] = struct{}{}
				facts = append(facts, FactBlock{Label: label, Value: value})
			}
			lineParts = append(lineParts, label+" "+value)
		}

		if i == 0 {
			draft.WriteString(strings.Join(lineParts, "；"))
			draft.WriteString("。")
		} else {
			draft.WriteString("\n- ")
			draft.WriteString(strings.Join(lineParts, "；"))
		}
	}
	return facts, draft.String()
}

// missionFieldPresence records which wanted fields any matched row carries.
func missionFieldPresence(matched []map[string]any, wanted []string) map[string]bool {
	haveField := map[string]bool{}
	for _, row := range matched {
		for _, field := range wanted {
			if _, v := missionField(row, field); v != "" {
				haveField[field] = true
			}
		}
	}
	return haveField
}

// buildMissionMissing lists wanted fields absent from the whole matched pool.
func buildMissionMissing(wanted []string, haveField map[string]bool) ([]string, []string) {
	missing := []string{}
	warnings := []string{}
	for _, field := range wanted {
		if haveField[field] {
			continue
		}
		label := missionFieldLabel(field)
		if label == "" {
			continue
		}
		missing = append(missing, label)
		warnings = append(warnings, "本地未收录: "+label)
	}
	return missing, warnings
}

// buildMissionDraft prefixes the draft body with the numeric range sentence.
func buildMissionDraft(rangeParts []string, body string) string {
	if len(rangeParts) == 0 {
		return body
	}
	out := strings.Join(rangeParts, "；") + "。"
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		out += "\n" + trimmed
	}
	return out
}

// missionTruncationWarning names the returned/limit ratio when available.
func missionTruncationWarning(meta map[string]any) string {
	total := formatNum(meta["total"])
	limit := formatNum(meta["limit"])
	msg := "任务列表已截断"
	if total != "" && limit != "" {
		msg += "（返回 " + limit + "/" + total + "）"
	}
	return msg + "，手续费/奖励区间可能不完整。"
}
