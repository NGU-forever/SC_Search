package router

import "testing"

func TestExpandDAGTrade(t *testing.T) {
	plan, err := ExpandDAG("黄金哪里卖", "trade", map[string]any{
		"commodity": "黄金",
		"side":      "sell",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Primary != "trade" {
		t.Fatalf("primary=%s", plan.Primary)
	}
	tools := map[string]bool{}
	for _, c := range plan.Calls {
		tools[c["tool"].(string)] = true
	}
	if !tools["sc_best_prices"] {
		t.Fatalf("expected sc_best_prices in %#v", plan.Calls)
	}
	if tools["sc_answer"] {
		t.Fatal("sc_answer should be skipped in worker DAG")
	}
	if tools["sc_terminal_prices"] {
		t.Fatal("terminal_prices should drop without terminal slot")
	}
}
