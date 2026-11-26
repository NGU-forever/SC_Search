package orchestrate

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/frankxiong/sc-search/internal/anyx"
)

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
			key := missionTitle(row) + "|" + anyx.FirstField(row, "canonical_name") + "|" + formatNum(row["cost_uec"]) + "|" + formatNum(row["reward_uec"])
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
		hay := strings.ToLower(missionTitle(row) + " " + anyx.FirstField(row, "canonical_name") + " " + anyx.FirstField(row, "mission_type"))
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
		if sysWant != "" && strings.Contains(strings.ToLower(anyx.FirstField(row, "systems")), sysWant) {
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
	return anyx.Unique(out)
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
