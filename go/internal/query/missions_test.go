package query_test

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

func TestListMissionsBasic(t *testing.T) {
	sqlDB := testfixture.New(t)

	list, err := query.ListMissions(sqlDB, "", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(list.Rows))
	}
	if _, ok := list.Rows[0]["canonical_name"]; !ok {
		t.Fatalf("missing canonical_name: %#v", list.Rows[0])
	}
	if _, ok := list.Rows[0]["entity_id"]; ok {
		t.Fatal("entity_id should be stripped from public rows")
	}
	if list.Total != 53 {
		t.Fatalf("total=%d want 53", list.Total)
	}
	if !list.Truncated {
		t.Fatal("expected truncated=true when total>limit")
	}
	if list.Limit != 5 {
		t.Fatalf("limit=%d", list.Limit)
	}
}

func TestListMissionsFactionFilter(t *testing.T) {
	sqlDB := testfixture.New(t)

	list, err := query.ListMissions(sqlDB, "Bit Zeros", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Rows) != 50 || list.Total != 51 {
		t.Fatalf("rows=%d total=%d want 50/51", len(list.Rows), list.Total)
	}
	if !list.Truncated {
		t.Fatal("50-row limit should truncate 51 Bit Zeros missions")
	}
	for _, row := range list.Rows {
		if got := row["contractor"]; got != "Bit Zeros" {
			t.Fatalf("contractor=%v want Bit Zeros", got)
		}
	}
}
