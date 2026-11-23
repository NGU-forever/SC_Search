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

	rows, err := ListMissions(sqlDB, "", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected some missions")
	}
	if _, ok := rows[0]["canonical_name"]; !ok {
		t.Fatalf("missing canonical_name: %#v", rows[0])
	}
	if _, ok := rows[0]["entity_id"]; ok {
		t.Fatal("entity_id should be stripped from public rows")
	}
}
