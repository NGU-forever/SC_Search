package query

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestResolveNameCarrackAlias(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	rows, err := ResolveName(sqlDB, "卡拉克")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected hits for 卡拉克")
	}
	found := false
	for _, r := range rows {
		if str(r["canonical_name"]) == "Carrack" || str(r["kind"]) == "ship" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected rows: %#v", rows[:min(3, len(rows))])
	}
}

func TestDataFreshness(t *testing.T) {
	root := repoRoot(t)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()
	rows, err := DataFreshness(sqlDB, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("expected ingest_runs")
	}
	live, err := LiveGameVersion(sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if live == "" {
		t.Fatal("empty live")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
