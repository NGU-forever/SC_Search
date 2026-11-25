package orchestrate

import (
	"strconv"
	"strings"
	"testing"
)

func TestMissionProjectorCostCooldown(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Bit Zeros ship heist 手续费和冷却",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_resolve_name",
				"data": []map[string]any{{"canonical_name": "Bit Zeros", "kind": "faction"}},
			},
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{
						"canonical_name": "BitZeros_BlackBoxRecovery_Nyx_Easy",
						"title":          "Getting To the Bottom of It",
						"contractor":     "Bit Zeros",
						"reward_uec":     39750,
						"cost_uec":       nil,
						"cooldown":       1,
						"systems":        "Nyx",
					},
					{
						"canonical_name": "UnafilliatedCFP_Pyro_RegionA_E_1AsteroidBase_Criminals_ShipHeist",
						"title":          "轻松拾取",
						"contractor":     "Bit Zeros",
						"cost_uec":       13500,
						"cooldown":       45,
						"lifetime":       1,
						"systems":        "Pyro",
						"reputation_min": 800,
						"reputation_rank": "Jr. Contractor",
					},
					{
						"canonical_name": "UnafilliatedCFP_Pyro_RegionA_M_2steroidBase_Criminals_ShipHeist",
						"title":          "有好货",
						"contractor":     "Bit Zeros",
						"cost_uec":       33000,
						"cooldown":       45,
						"systems":        "Pyro",
					},
				},
			},
		},
		Meta: map[string]any{"as_of": "4.10.3"},
	}

	next := present(st)
	if next != PhaseDone && next != PhaseAwaitUser {
		t.Fatalf("phase=%s", next)
	}
	text := st.Draft + " " + st.Blocks.Headline
	for _, f := range st.Blocks.Facts {
		text += " " + f.Label + " " + f.Value
	}
	if !strings.Contains(text, "13500") {
		t.Fatalf("missing fee 13500 in %q facts=%+v", st.Draft, st.Blocks.Facts)
	}
	if !strings.Contains(text, "45") || !strings.Contains(text, "分钟") {
		t.Fatalf("missing cooldown minutes in %q facts=%+v", st.Draft, st.Blocks.Facts)
	}
	if strings.Contains(st.Draft, "实体: Bit Zeros") && !strings.Contains(st.Draft, "手续费") {
		t.Fatalf("should not fall back to entity-only draft: %q", st.Draft)
	}
	// Prefer ship-heist row over blackbox recovery without fee.
	if strings.Contains(text, "Getting To the Bottom of It") && !strings.Contains(text, "轻松拾取") && !strings.Contains(text, "有好货") {
		t.Fatalf("should rank ship heist above unrelated missions: %q", text)
	}
}

func TestMissionProjectorCostOnly(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "这个任务手续费多少",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{
						"title":    "轻松拾取",
						"cost_uec": 13500,
						"cooldown": 45,
						"systems":  "Pyro",
					},
				},
			},
		},
	}
	_ = present(st)
	text := st.Draft
	for _, f := range st.Blocks.Facts {
		text += " " + f.Label + " " + f.Value
	}
	if !strings.Contains(text, "13500") {
		t.Fatalf("expected fee, got %q", text)
	}
	// Asking only fee should still allow cooldown if present is fine; ensure no fabricated fields.
	if strings.Contains(text, "<nil>") {
		t.Fatalf("nil leak: %q", text)
	}
}

func TestMissionProjectorNoCrossMix(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "ship heist 手续费和冷却",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "A", "canonical_name": "ShipHeist_A", "cost_uec": 100, "cooldown": 10, "systems": "Pyro"},
					{"title": "B", "canonical_name": "ShipHeist_B", "cost_uec": 200, "cooldown": 20, "systems": "Pyro"},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Draft, "100") {
		t.Fatalf("draft=%q", proj.Draft)
	}
	// First line should pair 100 with 10, not 100 with 20.
	first := strings.Split(proj.Draft, "\n")[0]
	if strings.Contains(first, "100") && strings.Contains(first, "20 分钟") {
		t.Fatalf("cross-mixed fields in first mission line: %q", first)
	}
}

func TestMissionProjectorFeeWithoutHeistKeyword(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Bit Zeros 任务手续费多少",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{
						"title":          "Bad at Goodbyes",
						"canonical_name": "BitZeros_BlackBoxRecovery_Nyx_Super",
						"contractor":     "Bit Zeros",
						"reward_uec":     101000,
						"cost_uec":       nil,
						"cooldown":       1,
						"systems":        "Nyx",
					},
					{
						"title":          "轻松拾取",
						"canonical_name": "UnafilliatedCFP_Pyro_RegionA_E_1AsteroidBase_Criminals_ShipHeist",
						"contractor":     "Bit Zeros",
						"cost_uec":       13500,
						"cooldown":       45,
						"systems":        "Pyro",
					},
				},
			},
		},
	}
	_ = present(st)
	text := st.Draft + " " + st.Blocks.Headline
	for _, f := range st.Blocks.Facts {
		text += " " + f.Label + " " + f.Value
	}
	if !strings.Contains(text, "13500") {
		t.Fatalf("fee question without heist keyword must prefer cost rows, got %q facts=%+v", st.Draft, st.Blocks.Facts)
	}
	if strings.Contains(st.Draft, "Bad at Goodbyes") && !strings.Contains(st.Draft, "13500") {
		t.Fatalf("must not lead with no-fee title-only answer: %q", st.Draft)
	}
}

func TestMissionProjectorMissingFeeWarning(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "某势力任务手续费多少",
		Slots:    map[string]any{"faction": "Example"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "Only Reward", "reward_uec": 1000, "cooldown": 1, "systems": "Stanton"},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	joined := strings.Join(proj.Warnings, ";") + " " + strings.Join(proj.Missing, ";")
	if !strings.Contains(joined, "手续费") {
		t.Fatalf("expected missing fee warning, got warnings=%v missing=%v draft=%q", proj.Warnings, proj.Missing, proj.Draft)
	}
}

func TestMissionProjectorRequiresFaction(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Zorblax 任务手续费多少",
		Slots:    map[string]any{},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "Adagio fee", "contractor": "Adagio Holdings", "cost_uec": 150, "cooldown": 1},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(strings.Join(proj.Missing, ","), "faction") {
		t.Fatalf("expected faction missing, got %+v", proj)
	}
	if strings.Contains(proj.Draft, "150") {
		t.Fatalf("must not answer with unfiltered fees: %q", proj.Draft)
	}
}

func TestMissionProjectorFeeAndRewardTogether(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Bit Zeros 任务手续费和报酬",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "轻松拾取", "cost_uec": 13500, "cooldown": 45, "reward_uec": nil, "systems": "Pyro"},
					{"title": "有好货", "cost_uec": 33000, "cooldown": 45, "reward_uec": nil, "systems": "Pyro"},
					{"title": "异种威胁", "cost_uec": 43000, "cooldown": 45, "reward_uec": nil, "systems": "Pyro"},
					{"title": "Bad at Goodbyes", "cost_uec": nil, "reward_uec": 101000, "systems": "Nyx"},
					{"title": "Data Transfer", "cost_uec": nil, "reward_uec": 33750, "systems": "Nyx"},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Headline, "13500") || !strings.Contains(proj.Headline, "43000") {
		t.Fatalf("fee range missing in headline: %q", proj.Headline)
	}
	if !strings.Contains(proj.Headline, "33750") || !strings.Contains(proj.Headline, "101000") {
		t.Fatalf("reward range missing in headline: %q", proj.Headline)
	}
	joinedWarn := strings.Join(proj.Warnings, ";")
	if strings.Contains(joinedWarn, "奖励") {
		t.Fatalf("must not claim reward missing when pool has rewards: %v draft=%q", proj.Warnings, proj.Draft)
	}
	if !strings.Contains(proj.Draft, "手续费") || !strings.Contains(proj.Draft, "奖励") {
		t.Fatalf("draft should mention both ranges: %q", proj.Draft)
	}
}

func TestMissionProjectorRepHeadlineNotFee(t *testing.T) {
	st := &State{
		Primary:  "mission_rep",
		Question: "Bit Zeros 声望怎么刷",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "Heist", "cost_uec": 13500, "cooldown": 45, "reputation_rank": "Jr. Contractor", "reputation_min": 800},
					{"title": "BlackBox", "reward_uec": 101000, "cooldown": 1, "reputation_rank": "Head Contractor", "reputation_min": 38000},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if strings.Contains(proj.Headline, "手续费") {
		t.Fatalf("rep question must not use fee headline: %q", proj.Headline)
	}
	if !strings.Contains(proj.Headline, "声望") && !strings.Contains(proj.Draft, "声望") {
		t.Fatalf("expected rep framing, headline=%q draft=%q", proj.Headline, proj.Draft)
	}
}

func TestMissionProjectorRewardHeadline(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "舒宾任务报酬多少",
		Slots:    map[string]any{"faction": "Shubin Interstellar"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "Low", "reward_uec": 203250, "systems": "Stanton"},
					{"title": "High", "reward_uec": 3071250, "systems": "Stanton"},
					{"title": "FeeOnly", "cost_uec": 150, "systems": "Stanton"},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Headline, "203250") || !strings.Contains(proj.Headline, "3071250") {
		t.Fatalf("reward headline range missing: %q", proj.Headline)
	}
	if strings.Contains(proj.Headline, "手续费") {
		t.Fatalf("reward question should not use fee headline: %q", proj.Headline)
	}
}

func TestMissionProjectorFeeHeadlineUsesFullPool(t *testing.T) {
	rows := []map[string]any{}
	for _, fee := range []int{150, 250, 1000, 5000, 20000} {
		rows = append(rows, map[string]any{
			"title":    "Fee " + strconv.Itoa(fee),
			"cost_uec": fee,
			"cooldown": 1,
			"systems":  "Stanton",
		})
	}
	st := &State{
		Primary:  "mission_money",
		Question: "Example 任务手续费",
		Slots:    map[string]any{"faction": "Example"},
		Evidence: []map[string]any{{"tool": "sc_list_missions", "data": rows}},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Headline, "150") || !strings.Contains(proj.Headline, "20000") {
		t.Fatalf("headline must use full fee pool range, got %q", proj.Headline)
	}
}

func TestMissionProjectorAdagioFeePreference(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Adagio Holdings 任务手续费",
		Slots:    map[string]any{"faction": "Adagio Holdings"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{
						"title":          "Adagio Holdings in Need of Salvagers",
						"canonical_name": "Adagio_Salvage_Intro",
						"contractor":     "Adagio Holdings",
						"reward_uec":     5000,
						"cost_uec":       nil,
						"cooldown":       1,
					},
					{
						"title":          "Paid Entry",
						"canonical_name": "Adaigo_Paid_Hard",
						"contractor":     "Adagio Holdings",
						"cost_uec":       15000,
						"cooldown":       30,
					},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Draft, "15000") {
		t.Fatalf("should prefer fee row even when title lacks faction words, draft=%q", proj.Draft)
	}
}

func TestMissionProjectorFeeHeadlineAndPlaceholders(t *testing.T) {
	st := &State{
		Primary:  "mission_money",
		Question: "Bit Zeros 任务手续费",
		Slots:    map[string]any{"faction": "Bit Zeros"},
		Evidence: []map[string]any{
			{
				"tool": "sc_list_missions",
				"data": []map[string]any{
					{"title": "轻松拾取", "cost_uec": 13500, "cooldown": 45, "systems": "Pyro"},
					{"title": "有好货", "cost_uec": 33000, "cooldown": 45, "systems": "Pyro"},
					{"title": "Claim #[CLAIM]: [SHIP] Salvage Rights", "cost_uec": 20000, "cooldown": 30, "systems": "Stanton"},
				},
			},
		},
	}
	proj := missionProjector{}.Project(st)
	if !strings.Contains(proj.Headline, "13500") || !strings.Contains(proj.Headline, "33000") {
		t.Fatalf("headline should show fee range, got %q", proj.Headline)
	}
	if strings.HasPrefix(proj.Draft, "有好货") {
		t.Fatalf("should lead with lowest fee row, draft=%q", proj.Draft)
	}
	if strings.Contains(proj.Draft, "[CLAIM]") || strings.Contains(proj.Draft, "[SHIP]") {
		t.Fatalf("placeholders should be cleaned, draft=%q", proj.Draft)
	}
	if strings.Contains(proj.Draft, " : ") {
		t.Fatalf("stray colon spacing left in draft=%q", proj.Draft)
	}
	if !strings.Contains(proj.Draft, "Salvage Rights") {
		t.Fatalf("cleaned title should remain, draft=%q", proj.Draft)
	}
}
