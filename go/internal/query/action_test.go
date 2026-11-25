package query

import "testing"

func TestDetectActionPriority(t *testing.T) {
	cases := []struct {
		q      string
		action Action
		rule   string
	}{
		{"怎么采矿配激光", ActionMining, "mining_setup"},
		{"采矿船货舱多大", ActionFacts, "facts_fallback"}, // static mining params are not mining_setup
		{"流星改装方案", ActionLoadout, "loadout"},
		{"RSI Meteor loadout", ActionLoadout, "loadout"},
		{"Bit Zeros ship heist 手续费和冷却", ActionMissionMoney, "mission_money"},
		{"舒宾任务报酬多少", ActionMissionMoney, "mission_money"},
		{"红风 Junior 能接什么任务", ActionMissionList, "mission_list"},
		{"Bit Zeros 声望怎么刷", ActionMissionList, "mission_list"},
		{"黄金哪里卖最贵", ActionPrice, "price"},
		{"汉化进不去游戏怎么办", ActionClient, "client"},
		{"CCU 是什么", ActionPledge, "pledge"},
		{"Invictus 活动怎么玩", ActionEvent, "event"},
		{"萌新怎么赚钱开荒", ActionEarn, "earn"},
		{"怎么叫船", ActionHowto, "howto"},
		{"跑商航线推荐", ActionTradeRoute, "trade_route"},
		{"Carrack 在哪买", ActionPrice, "price"}, // price keywords fire; ship_buy is entity-gated in QueryPlan
		{"当前 Live 版本", ActionLiveLocal, "live_local"},
		{"本地数据是什么时候同步的？", ActionDataSync, "data_sync"},
	}
	for _, c := range cases {
		got := DetectAction(c.q)
		if got.Action != c.action {
			t.Errorf("%q: action=%s want=%s rule=%s", c.q, got.Action, c.action, got.RuleID)
		}
		if c.rule != "" && got.RuleID != c.rule {
			t.Errorf("%q: rule=%s want=%s", c.q, got.RuleID, c.rule)
		}
	}
}

func TestDetectActionMiningBeatsLoadoutWords(t *testing.T) {
	// mining setup must win over generic 配装 in the same sentence.
	got := DetectAction("怎么采矿配激光")
	if got.Action != ActionMining {
		t.Fatalf("got %s (%s)", got.Action, got.RuleID)
	}
}

func TestDetectActionMissionMoneyBeatsPrice(t *testing.T) {
	got := DetectAction("任务要交多少钱")
	if got.Action != ActionMissionMoney {
		t.Fatalf("got %s (%s)", got.Action, got.RuleID)
	}
}
