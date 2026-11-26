package query_test

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

func TestResolveNameCarrackAlias(t *testing.T) {
	sqlDB := testfixture.New(t)

	rows, err := query.ResolveName(sqlDB, "卡拉克")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected hits for 卡拉克")
	}
	found := false
	for _, r := range rows {
		if r["canonical_name"] == "Anvil Carrack" && r["kind"] == "ship" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected rows: %#v", rows[:min(3, len(rows))])
	}
}

func TestResolveNameCategoryLabelDoesNotHitFleet(t *testing.T) {
	sqlDB := testfixture.New(t)

	rows, err := query.ResolveName(sqlDB, "勘探")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r["kind"] == "ship" {
			t.Fatalf("勘探 resolved to ship %v; SCW category labels must be facts, not aliases", r["canonical_name"])
		}
	}
}

func TestDataFreshness(t *testing.T) {
	sqlDB := testfixture.New(t)

	rows, err := query.DataFreshness(sqlDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected ingest_runs")
	}
	live, err := query.LiveGameVersion(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if live != "4.10.3" {
		t.Fatalf("live=%q want 4.10.3", live)
	}
}
