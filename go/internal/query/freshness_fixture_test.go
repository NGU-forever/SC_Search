package query_test

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

func TestAnnotateFreshnessRows(t *testing.T) {
	conn := testfixture.NewWithOptions(t, testfixture.Options{UEXGameVersion: testfixture.StaleUEXVersion})

	rows, err := query.DataFreshness(conn, "")
	if err != nil {
		t.Fatal(err)
	}
	annotated, meta := query.AnnotateFreshnessRows(conn, rows)
	if len(annotated) == 0 {
		t.Fatal("no freshness rows")
	}
	if meta["as_of"] == nil || meta["as_of"] == "" {
		t.Fatalf("missing as_of: %#v", meta)
	}
	uexStale := false
	for _, row := range annotated {
		src, _ := row["source"].(string)
		stale, _ := row["stale"].(bool)
		comparable, _ := row["version_comparable"].(bool)
		if src == "aliases" || src == "localization" {
			if stale || comparable {
				t.Fatalf("%s should not be comparable/stale vs Live: %#v", src, row)
			}
		}
		if src == "uex" {
			if !stale || !comparable {
				t.Fatalf("stale fixture must mark uex stale: %#v", row)
			}
			uexStale = true
		}
	}
	if !uexStale {
		t.Fatal("missing uex freshness row")
	}
	staleSources, _ := meta["stale_sources"].([]string)
	if len(staleSources) != 1 || staleSources[0] != "uex" {
		t.Fatalf("stale_sources=%v", staleSources)
	}
}
