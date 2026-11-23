package query

import "testing"

func TestIntentsCatalog(t *testing.T) {
	cat, err := IntentsCatalog()
	if err != nil {
		t.Fatal(err)
	}
	primaries, ok := cat["primaries"].([]any)
	if !ok || len(primaries) < 10 {
		t.Fatalf("bad primaries: %#v", cat["primaries"])
	}
	playbooks, ok := cat["playbooks"].(map[string]any)
	if !ok || playbooks["trade"] == nil {
		t.Fatalf("missing trade playbook")
	}
}
