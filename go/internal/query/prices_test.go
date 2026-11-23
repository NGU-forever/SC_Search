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
