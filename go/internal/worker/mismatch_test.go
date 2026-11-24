package worker

import "testing"

func TestFamiliesPresentSkipsWeakAugment(t *testing.T) {
	rows := []map[string]any{
		{
			"tool": "sc_search_docs", "ok": true, "data": []any{},
			"meta": map[string]any{"why": "no_match"},
		},
		{
			"tool": "sc_answer", "ok": true,
			"data":            map[string]any{"answer_draft": "generic howto"},
			"augmented":       true,
			"evidence_weight": 0,
		},
	}
	present := familiesPresent(rows)
	if len(present) != 0 {
		t.Fatalf("weak augment must not populate families: %#v", present)
	}
	if !StrongFamilyMissing("howto", rows) {
		t.Fatal("howto with only weak answer should report strong family missing")
	}
}

func TestFamiliesPresentCountsResolveFactsAugment(t *testing.T) {
	rows := []map[string]any{
		{
			"tool": "sc_entity_facts", "ok": true,
			"data":            map[string]any{"entity": map[string]any{"kind": "ship"}},
			"augmented":       true,
			"evidence_weight": 1,
		},
	}
	if StrongFamilyMissing("ship_stats", rows) {
		t.Fatal("resolve→facts augment should satisfy ship_stats strong family")
	}
}

func TestAggregateCallMetaStale(t *testing.T) {
	rows := []map[string]any{
		{
			"tool": "sc_best_prices", "ok": true,
			"meta": map[string]any{"stale": true, "game_version": "3.24.2", "as_of": "4.10.3"},
		},
		{
			"tool": "sc_resolve_name", "ok": true,
			"meta": map[string]any{},
		},
	}
	meta := AggregateCallMeta(rows)
	if meta["stale"] != true {
		t.Fatalf("expected stale=true, got %#v", meta)
	}
	if meta["as_of"] != "4.10.3" {
		t.Fatalf("as_of=%v", meta["as_of"])
	}
}
