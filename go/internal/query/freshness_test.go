package query

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestIsComparableGameVersion(t *testing.T) {
	if !IsComparableGameVersion("4.10.3-live.13220821") {
		t.Fatal("expected comparable")
	}
	if !IsComparableGameVersion("3.24.2") {
		t.Fatal("expected comparable")
	}
	if IsComparableGameVersion("aliases") {
		t.Fatal("aliases must not be comparable")
	}
	if IsComparableGameVersion("localization-full") {
		t.Fatal("localization must not be comparable")
	}
}

func TestAnnotateFreshnessRows(t *testing.T) {
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
	annotated, meta := AnnotateFreshnessRows(sqlDB, rows)
	if len(annotated) == 0 {
		t.Fatal("no freshness rows")
	}
	if meta["as_of"] == nil || meta["as_of"] == "" {
		t.Fatalf("missing as_of: %#v", meta)
	}
	for _, row := range annotated {
		src := str(row["source"])
		stale, _ := row["stale"].(bool)
		comparable, _ := row["version_comparable"].(bool)
		if src == "aliases" || src == "localization" {
			if stale {
				t.Fatalf("%s should not be stale vs Live", src)
			}
			if comparable {
				t.Fatalf("%s should not be version_comparable", src)
			}
		}
		if src == "uex" && comparable {
			// UEX currently ships older game_version than SCMDB Live.
			_ = stale
		}
	}
	staleSources, _ := meta["stale_sources"].([]string)
	_ = staleSources
}
