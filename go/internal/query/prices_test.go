package query_test

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

func TestBestPricesGold(t *testing.T) {
	sqlDB := testfixture.New(t)

	rows, err := query.BestPrices(sqlDB, "黄金", "sell", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected gold sell prices")
	}
	if rows[0]["commodity"] == nil || rows[0]["terminal"] == nil {
		t.Fatalf("incomplete row: %#v", rows[0])
	}
	if got := rows[0]["price"]; got != 9500.0 {
		t.Fatalf("top gold sell=%v want 9500", got)
	}
}

func TestBestPricesSystemFilterStanton(t *testing.T) {
	sqlDB := testfixture.New(t)

	rows, err := query.BestPrices(sqlDB, "Gold", "sell", "Stanton", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected Stanton Gold sell prices")
	}
	if len(rows) != 1 {
		t.Fatalf("want only Area 18 in Stanton, got %d rows", len(rows))
	}
	for _, r := range rows {
		term, _ := r["terminal"].(string)
		if !query.TerminalMatchesSystem(term, "Stanton") {
			t.Fatalf("terminal %q should match Stanton", term)
		}
	}
}

func TestTerminalMatchesSystem(t *testing.T) {
	if !query.TerminalMatchesSystem("TDD - Trade and Development Division - Area 18", "Stanton") {
		t.Fatal("Area 18 should be Stanton")
	}
	if query.TerminalMatchesSystem("TDD - Trade and Development Division - Area 18", "Pyro") {
		t.Fatal("Area 18 should not match Pyro")
	}
	if !query.TerminalMatchesSystem("Admin - Nyx Gateway (Pyro)", "Pyro") {
		t.Fatal("parenthetical Pyro should match")
	}
}

func TestEntityFactsCarrack(t *testing.T) {
	sqlDB := testfixture.New(t)

	bundle, err := query.EntityFacts(sqlDB, "卡拉克", "full")
	if err != nil {
		t.Fatal(err)
	}
	if bundle == nil {
		t.Fatal("nil bundle")
	}
	ent, _ := bundle["entity"].(map[string]any)
	if got, _ := ent["canonical_name"].(string); got != "Anvil Carrack" {
		t.Fatalf("canonical_name=%q", got)
	}
}
