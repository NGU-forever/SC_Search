package query

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestListMissionsBasic(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	list, err := ListMissions(sqlDB, "", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Rows) == 0 {
		t.Fatal("expected some missions")
	}
	if _, ok := list.Rows[0]["canonical_name"]; !ok {
		t.Fatalf("missing canonical_name: %#v", list.Rows[0])
	}
	if _, ok := list.Rows[0]["entity_id"]; ok {
		t.Fatal("entity_id should be stripped from public rows")
	}
	if list.Total < len(list.Rows) {
		t.Fatalf("total %d < rows %d", list.Total, len(list.Rows))
	}
	if list.Limit != 5 {
		t.Fatalf("limit=%d", list.Limit)
	}
	if list.Total > 5 && !list.Truncated {
		t.Fatal("expected truncated=true when total>limit")
	}
}
