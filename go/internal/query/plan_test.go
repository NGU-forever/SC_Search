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

func planRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
