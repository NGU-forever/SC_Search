package query_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
	"github.com/frankxiong/sc-search/internal/query"
)

// TestLiveDBSmoke is the only test that needs data/sc_knowledge.sqlite. It is
// supplementary: fixture tests cover CI, and this skips with a clear reason when
// the live DB is absent.
func TestLiveDBSmoke(t *testing.T) {
	root := liveRepoRoot(t)
	conn, err := db.Open(root)
	if err != nil {
		t.Skipf("live DB not present (%v); fixture tests cover CI", err)
	}
	defer conn.Close()

	rows, err := query.DataFreshness(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("live DB has no ingest_runs")
	}
	live, err := query.LiveGameVersion(conn)
	if err != nil {
		t.Fatal(err)
	}
	if live == "" {
		t.Fatal("live DB has no Live game version")
	}
}

func liveRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
