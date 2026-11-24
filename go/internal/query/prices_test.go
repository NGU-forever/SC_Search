package query

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestBestPricesGold(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	rows, err := BestPrices(sqlDB, "黄金", "sell", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected gold sell prices")
	}
	if rows[0]["commodity"] == nil || rows[0]["terminal"] == nil {
		t.Fatalf("incomplete row: %#v", rows[0])
	}
}

func TestBestPricesSystemFilterStanton(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	rows, err := BestPrices(sqlDB, "Agricium", "sell", "Stanton", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected Stanton Agricium sell prices (Area 18 / Lorville POI match)")
	}
	for _, r := range rows {
		term := str(r["terminal"])
		if !TerminalMatchesSystem(term, "Stanton") {
			t.Fatalf("terminal %q should match Stanton", term)
		}
	}
}

func TestTerminalMatchesSystem(t *testing.T) {
	if !TerminalMatchesSystem("TDD - Trade and Development Division - Area 18", "Stanton") {
		t.Fatal("Area 18 should be Stanton")
	}
	if TerminalMatchesSystem("TDD - Trade and Development Division - Area 18", "Pyro") {
		t.Fatal("Area 18 should not match Pyro")
	}
	if !TerminalMatchesSystem("Admin - Nyx Gateway (Pyro)", "Pyro") {
		t.Fatal("parenthetical Pyro should match")
	}
}

func TestEntityFactsCarrack(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	bundle, err := EntityFacts(sqlDB, "卡拉克", "full")
	if err != nil {
		t.Fatal(err)
	}
	if bundle == nil {
		t.Fatal("nil bundle")
	}
	ent := bundle["entity"].(map[string]any)
	if str(ent["canonical_name"]) == "" {
		t.Fatalf("bad entity %#v", ent)
	}
}
