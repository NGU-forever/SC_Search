package orchestrate

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type missionProjector struct{}

func (missionProjector) Supports(primary string) bool {
	return primary == "mission_money" || primary == "mission_rep"
}

func (missionProjector) Project(st *State) Projection {
	faction := asString(st.Slots["faction"])
	if faction == "" {
		// Never answer mission fees from an unfiltered global mission dump.
		return Projection{
			Headline: "缺少势力",
			Draft:    "未能识别任务势力；请使用标准势力名（如 Shubin Interstellar / Bit Zeros）后再查手续费或冷却。",
			Missing:  []string{"faction"},
			Warnings: []string{"本地未识别势力，勿使用未过滤任务列表作答。"},
		}
	}

	rows := evidenceByTool(st, "sc_list_missions")
	if len(rows) == 0 {
		return Projection{
			Headline: "势力: " + faction,
			Draft:    "势力 " + faction + " 本地未命中任务列表。",
			Missing:  []string{"missions"},
			Warnings: []string{"本地未收录: missions"},
		}
	}

	wanted := missionWantedFields(st.Question)
	matched := rankMissionRows(rows, st, wanted)
	if len(matched) == 0 {
		matched = preferRowsWithWanted(rows, wanted)
	}
	if len(matched) == 0 {
		matched = rows
	}

	feePool := preferRowsWithWanted(matched, []string{"cost"})
	rewardPool := preferRowsWithWanted(matched, []string{"reward"})
	if containsStr(wanted, "cost") {
		sortMissionRowsByCostAsc(feePool)
	}
	if containsStr(wanted, "reward") {
		sortMissionRowsByRewardDesc(rewardPool)
	}

	// Missing fields must be judged against the full matched pool, not the 3 display rows.
	haveField := map[string]bool{}
	for _, row := range matched {
		for _, field := range wanted {
			if _, v := missionField(row, field); v != "" {
				haveField[field] = true
			}
		}
	}

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
	headline := "任务机制"
	if len(rangeParts) > 0 {
		headline = strings.Join(rangeParts, "；")
	} else if containsStr(wanted, "rep") {
		headline = "声望任务: " + faction
	} else if title := cleanMissionTitle(missionTitle(matched[0])); title != "" {
		headline = title
	}

	display := mixMissionDisplay(matched, feePool, rewardPool, wanted, 3)

	facts := []FactBlock{}
	missing := []string{}
	warnings := []string{}
	var draft strings.Builder
	facts = append(facts, FactBlock{Label: "势力", Value: faction})

	seenFact := map[string]struct{}{}
	for i, row := range display {
		title := cleanMissionTitle(missionTitle(row))
		if title == "" {
			continue
		}
		sys := firstNonEmpty(row, "systems")

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
	if len(rangeParts) > 0 {
		body := strings.TrimSpace(draft.String())
		draft.Reset()
		draft.WriteString(strings.Join(rangeParts, "；"))
		draft.WriteString("。")
		if body != "" {
			draft.WriteString("\n")
			draft.WriteString(body)
		}
	}

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

	if meta := evidenceMetaByTool(st, "sc_list_missions"); asBool(meta["truncated"]) {
		total := formatNum(meta["total"])
		limit := formatNum(meta["limit"])
		msg := "任务列表已截断"
		if total != "" && limit != "" {
			msg += "（返回 " + limit + "/" + total + "）"
		}
		msg += "，手续费/奖励区间可能不完整。"
		warnings = appendUnique(warnings, msg)
		if len(rangeParts) > 0 && !strings.Contains(headline, "截断") {
			headline = headline + "（列表截断，可能不完整）"
		}
	}

	const missionFactCap = 12
	if len(facts) > missionFactCap {
		facts = prioritizeMissionFacts(facts, wanted)
		if len(facts) > missionFactCap {
			facts = facts[:missionFactCap]
		}
	}

	return Projection{
		Headline: headline,
		Facts:    facts,
		Draft:    strings.TrimSpace(draft.String()),
		Missing:  missing,
		Warnings: warnings,
	}
}

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
		if rank := firstNonEmpty(row, "reputation_rank"); rank != "" {
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
		if r := firstNonEmpty(row, "pyro_regions"); r != "" {
			parts = append(parts, "地区 "+r)
		}
		if s := firstNonEmpty(row, "site_pool"); s != "" {
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

// mixMissionDisplay picks up to limit rows, mixing fee and reward sets when both are asked.
func mixMissionDisplay(matched, feePool, rewardPool []map[string]any, wanted []string, limit int) []map[string]any {
	if limit <= 0 {
		limit = 3
	}
	wantCost := containsStr(wanted, "cost")
	wantReward := containsStr(wanted, "reward")
	if wantCost && wantReward && (len(feePool) > 0 || len(rewardPool) > 0) {
		out := []map[string]any{}
		seen := map[string]struct{}{}
		add := func(row map[string]any) {
			if len(out) >= limit {
				return
			}
			key := missionTitle(row) + "|" + firstNonEmpty(row, "canonical_name") + "|" + formatNum(row["cost_uec"]) + "|" + formatNum(row["reward_uec"])
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			out = append(out, row)
		}
		// Interleave: one fee, one reward, …
		i, j := 0, 0
		for len(out) < limit && (i < len(feePool) || j < len(rewardPool)) {
			if i < len(feePool) {
				add(feePool[i])
				i++
			}
			if len(out) >= limit {
				break
			}
			if j < len(rewardPool) {
				add(rewardPool[j])
				j++
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if wantCost && len(feePool) > 0 {
		if len(feePool) > limit {
			return feePool[:limit]
		}
		return feePool
	}
	if wantReward && len(rewardPool) > 0 {
		if len(rewardPool) > limit {
			return rewardPool[:limit]
		}
		return rewardPool
	}
	if len(matched) > limit {
		return matched[:limit]
	}
	return matched
}

func preferRowsWithWanted(rows []map[string]any, wanted []string) []map[string]any {
	if len(wanted) == 0 {
		return nil
	}
	out := []map[string]any{}
	for _, row := range rows {
		ok := false
		for _, field := range wanted {
			if _, v := missionField(row, field); v != "" {
				ok = true
				break
			}
		}
		if ok {
			out = append(out, row)
		}
	}
	return out
}

func rankMissionRows(rows []map[string]any, st *State, wanted []string) []map[string]any {
	activity := strings.ToLower(asString(st.Slots["activity"]))
	tokens := missionQueryTokens(st.Question, activity, st)
	wantCost := containsStr(wanted, "cost")
	wantCooldown := containsStr(wanted, "cooldown")
	wantReward := containsStr(wanted, "reward")

	// Prefer rows that carry the fields the user asked about.
	pool := rows
	if wantCost || wantCooldown || wantReward {
		filtered := []map[string]any{}
		for _, row := range rows {
			has := false
			if wantCost && formatNum(row["cost_uec"]) != "" {
				has = true
			}
			if wantReward && formatNum(row["reward_uec"]) != "" {
				has = true
			}
			if wantCooldown && formatNum(row["cooldown"]) != "" {
				if !wantCost || formatNum(row["cost_uec"]) != "" {
					has = true
				}
			}
			if has {
				filtered = append(filtered, row)
			}
		}
		if len(filtered) > 0 {
			pool = filtered
		}
	}

	type scored struct {
		score int
		row   map[string]any
	}
	var ranked []scored
	for _, row := range pool {
		score := 0
		hay := strings.ToLower(missionTitle(row) + " " + firstNonEmpty(row, "canonical_name") + " " + firstNonEmpty(row, "mission_type"))
		for _, tok := range tokens {
			if tok != "" && strings.Contains(strings.ReplaceAll(hay, " ", ""), tok) {
				score += 20
			}
		}
		for _, field := range wanted {
			if _, v := missionField(row, field); v != "" {
				score += 50
			}
		}
		sysWant := strings.ToLower(asString(st.Slots["system"]))
		if sysWant != "" && strings.Contains(strings.ToLower(firstNonEmpty(row, "systems")), sysWant) {
			score += 5
		}
		ranked = append(ranked, scored{score: score, row: row})
	}
	if len(ranked) == 0 {
		return nil
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	out := make([]map[string]any, 0, len(ranked))
	for _, s := range ranked {
		out = append(out, s.row)
	}
	return out
}

var missionTokenStop = map[string]struct{}{
	"bit": {}, "zeros": {}, "zero": {}, "task": {}, "mission": {}, "missions": {},
	"fee": {}, "fees": {}, "cost": {}, "cooldown": {}, "cool": {}, "down": {},
	"what": {}, "where": {}, "when": {}, "how": {}, "much": {}, "many": {},
	"holdings": {}, "interstellar": {}, "adagio": {}, "shubin": {}, "rayari": {},
	"faction": {}, "contract": {}, "contracts": {}, "with": {}, "from": {}, "this": {},
	"that": {}, "have": {}, "need": {}, "want": {}, "please": {},
}

func missionQueryTokens(question, activity string, st *State) []string {
	raw := strings.ToLower(question + " " + activity)
	cands := []string{
		"shipheist", "ship heist", "heist", "黑吃黑", "抢船",
		"blackbox", "black box", "delivery", "mercenary", "salvage", "bounty",
	}
	out := []string{}
	for _, c := range cands {
		if strings.Contains(raw, c) {
			out = append(out, strings.ReplaceAll(c, " ", ""))
		}
	}

	// Drop faction/slot words so they cannot outrank mechanism fields.
	exclude := map[string]struct{}{}
	for _, stop := range []string{asString(st.Slots["faction"]), asString(st.Slots["system"])} {
		for _, tok := range splitAlphaNum(strings.ToLower(stop)) {
			exclude[tok] = struct{}{}
		}
	}
	for tok := range missionTokenStop {
		exclude[tok] = struct{}{}
	}

	for _, tok := range splitAlphaNum(raw) {
		if len(tok) < 4 {
			continue
		}
		if _, bad := exclude[tok]; bad {
			continue
		}
		// Skip pure Chinese stop-ish short activity words handled via cands.
		out = append(out, tok)
	}
	return uniqueStrings(out)
}

func splitAlphaNum(s string) []string {
	var b strings.Builder
	out := []string{}
	flush := func() {
		tok := strings.ToLower(b.String())
		b.Reset()
		if tok != "" {
			out = append(out, tok)
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
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

func missionTitle(row map[string]any) string {
	if t := firstNonEmpty(row, "title"); t != "" {
		return t
	}
	return firstNonEmpty(row, "canonical_name")
}

func cleanMissionTitle(title string) string {
	if title == "" {
		return ""
	}
	// Drop unfilled SCMDB placeholders like [CLAIM] / [SHIP] / #[CLAIM].
	out := placeholderRE.ReplaceAllString(title, " ")
	out = strings.ReplaceAll(out, " : ", " ")
	out = strings.ReplaceAll(out, ": ", " ")
	out = strings.Join(strings.Fields(out), " ")
	out = strings.TrimSpace(strings.Trim(out, ":-–— "))
	if out == "" {
		return title
	}
	return out
}

var placeholderRE = regexp.MustCompile(`#?\[[A-Za-z_]+\]`)

func feeHeadline(rows []map[string]any) string {
	return numericRangeHeadline(rows, "cost_uec", "手续费")
}

func rewardHeadline(rows []map[string]any) string {
	return numericRangeHeadline(rows, "reward_uec", "奖励")
}

func numericRangeHeadline(rows []map[string]any, key, label string) string {
	var nums []int64
	for _, row := range rows {
		v := formatNum(row[key])
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return ""
	}
	min, max := nums[0], nums[0]
	for _, n := range nums[1:] {
		if n < min {
			min = n
		}
		if n > max {
			max = n
		}
	}
	if min == max {
		return label + ": " + strconv.FormatInt(min, 10) + " aUEC"
	}
	return label + ": " + strconv.FormatInt(min, 10) + "–" + strconv.FormatInt(max, 10) + " aUEC"
}

func sortMissionRowsByCostAsc(rows []map[string]any) {
	sortMissionRowsByNum(rows, "cost_uec", true)
}

func sortMissionRowsByRewardDesc(rows []map[string]any) {
	sortMissionRowsByNum(rows, "reward_uec", false)
}

func sortMissionRowsByNum(rows []map[string]any, key string, asc bool) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			ai := rowNum(rows[i], key)
			aj := rowNum(rows[j], key)
			if ai < 0 && aj >= 0 {
				rows[i], rows[j] = rows[j], rows[i]
				continue
			}
			if ai >= 0 && aj >= 0 {
				if asc && aj < ai {
					rows[i], rows[j] = rows[j], rows[i]
				}
				if !asc && aj > ai {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
		}
	}
}

func rowNum(row map[string]any, key string) int64 {
	v := formatNum(row[key])
	if v == "" {
		return -1
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func firstNonEmpty(row map[string]any, keys ...string) string {
	for _, k := range keys {
		if s := asString(row[k]); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "…"
}

func uniqueStrings(xs []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, x := range xs {
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}
