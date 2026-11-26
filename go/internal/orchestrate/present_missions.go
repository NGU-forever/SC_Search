package orchestrate

import (
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
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

	rows := selectMissionRows(st)
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

	rangeParts := missionRangeParts(feePool, rewardPool, wanted)
	headline := buildMissionHeadline(faction, matched, wanted, rangeParts)
	display := mixMissionDisplay(matched, feePool, rewardPool, wanted, 3)
	facts, body := buildMissionFacts(faction, display, wanted)
	missing, warnings := buildMissionMissing(wanted, missionFieldPresence(matched, wanted))
	draft := buildMissionDraft(rangeParts, body)

	if meta := evidenceMetaByTool(st, "sc_list_missions"); anyx.Bool(meta["truncated"]) {
		msg := missionTruncationWarning(meta)
		warnings = anyx.AppendUnique(warnings, msg)
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
		Draft:    strings.TrimSpace(draft),
		Missing:  missing,
		Warnings: warnings,
	}
}

// selectMissionRows pulls the mission rows collected by sc_list_missions.
func selectMissionRows(st *State) []map[string]any {
	return evidenceByTool(st, "sc_list_missions")
}
