package query

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestQueryPlanPrices(t *testing.T) {
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	plan, err := QueryPlan(sqlDB, "黄金哪里卖最贵", "")
	if err != nil {
		t.Fatal(err)
	}
	if plan["intent"] != "prices" {
		t.Fatalf("intent=%v plan=%#v", plan["intent"], plan)
	}
}

func TestQueryPlanFactsCarrack(t *testing.T) {
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	plan, err := QueryPlan(sqlDB, "卡拉克货舱多大", "")
	if err != nil {
		t.Fatal(err)
	}
	if plan["intent"] != "facts" {
		t.Fatalf("intent=%v", plan["intent"])
	}
}

func TestQueryPlanActionIntents(t *testing.T) {
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	cases := []struct {
		q, intent string
	}{
		{"流星改装方案", "loadout_calc"},
		{"RSI Meteor loadout", "loadout_calc"},
		{"怎么采矿配激光", "mining_calc"},
		{"Bit Zeros ship heist 手续费和冷却", "mission_money"},
		{"Adagio Holdings 任务要交多少钱", "mission_money"},
		{"Bit Zeros 声望怎么刷", "mission_list"},
		{"红风 Junior 能接什么任务", "mission_list"},
		{"CCU 是什么", "pledge_meta"},
		{"Invictus 活动怎么玩", "event_guide"},
		{"Carrack 在哪买", "ship_shop"},
		{"当前 Live 版本", "live_local"},
		{"本地数据是什么时候同步的？", "data_sync"},
		{"黄金在 Area18 什么价", "prices"},
		// anti-examples
		{"采矿船货舱多大", "facts"},
		{"Carrack 火力多大", "facts"},
		{"黄金是什么", "facts"},
	}
	for _, c := range cases {
		plan, err := QueryPlan(sqlDB, c.q, "")
		if err != nil {
			t.Fatalf("%q: %v", c.q, err)
		}
		if plan["intent"] != c.intent {
			t.Errorf("%q intent=%v want=%s", c.q, plan["intent"], c.intent)
		}
	}
}

func TestSuggestedPrimaryContext(t *testing.T) {
	if got := SuggestedPrimary(ClassificationContext{
		Intent: "mission_list", Question: "红风 Junior 能接什么任务",
		Entities: []map[string]any{{"kind": "faction", "canonical_name": "Red Wind Linehaul"}},
	}); got != "mission_rep" {
		t.Fatalf("got %s", got)
	}
	if got := SuggestedPrimary(ClassificationContext{
		Intent: "facts", Question: "视差步枪是什么",
		Entities: []map[string]any{{"kind": "blueprint", "canonical_name": "Parallax Energy Assault Rifle"}},
	}); got != "other" {
		t.Fatalf("got %s", got)
	}
	if got := SuggestedPrimary(ClassificationContext{
		Intent: "facts", Question: "Carrack 货舱",
		Entities: []map[string]any{{"kind": "ship", "canonical_name": "Anvil Carrack"}},
	}); got != "ship_stats" {
		t.Fatalf("got %s", got)
	}
	if got := SuggestedPrimary(ClassificationContext{Intent: "pledge_meta"}); got != "pledge_meta" {
		t.Fatalf("got %s", got)
	}
	if got := SuggestedPrimary(ClassificationContext{Intent: "live_local"}); got != "other" {
		t.Fatalf("got %s", got)
	}
}

func planRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
