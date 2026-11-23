package worker

import "testing"

func TestBindArgsCanonical(t *testing.T) {
	results := map[string]map[string]any{
		"r1": {
			"ok": true,
			"data": []any{
				map[string]any{"canonical_name": "Carrack", "kind": "ship"},
			},
		},
	}
	out := bindArgs(map[string]any{
		"name_or_id": "$r1.canonical",
		"keep":       "x",
	}, results)
	if out["name_or_id"] != "Carrack" {
		t.Fatalf("got %#v", out["name_or_id"])
	}
	if out["keep"] != "x" {
		t.Fatal("keep mutated")
	}
}
