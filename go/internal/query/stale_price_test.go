package query_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankxiong/sc-search/internal/ingest"
	"github.com/frankxiong/sc-search/internal/query"
)

func TestIsStaleVsLive(t *testing.T) {
	cases := []struct {
		live, src string
		want      bool
	}{
		{"4.10.3", "3.24.2", true},
		{"4.10.3", "4.10.3", false},
		{"4.10.3", "4.10.3-live.123", false},
		{"4.10.3", "uex-unknown", false},
		{"4.10.3", "aliases", false},
		{"", "3.24.2", false},
	}
	for _, tc := range cases {
		if got := query.IsStaleVsLive(tc.live, tc.src); got != tc.want {
			t.Errorf("IsStaleVsLive(%q,%q)=%v want %v", tc.live, tc.src, got, tc.want)
		}
	}
}

func staleTestDB(t *testing.T, uexVersion string) *sql.DB {
	t.Helper()
	conn, err := ingest.Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := ingest.InitDB(conn); err != nil {
		t.Fatal(err)
	}
	// Live reference comes from the latest ok scmdb run.
	if _, err := conn.Exec(`
		INSERT INTO ingest_runs (source, game_version, started_at, finished_at, status, row_count)
		VALUES ('scmdb', '4.10.3-live.1', '2026-01-01T00:00:00Z', '2026-01-01T00:01:00Z', 'ok', 1)
	`); err != nil {
		t.Fatal(err)
	}
	_, err = ingest.SyncUexPayload(conn, ingest.UexPayload{
		GameVersion: uexVersion,
		Terminals:   []map[string]any{{"id": 1.0, "name": "Area18", "code": "A18"}},
		Commodities: []map[string]any{{"id": 10.0, "name": "Quantanium", "slug": "quantanium"}},
		Prices:      []map[string]any{{"id_commodity": 10.0, "id_terminal": 1.0, "price_sell": 80.0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestPriceAnswerStaleSnapshotNeverCurrentBest(t *testing.T) {
	conn := staleTestDB(t, "3.24.2")
	ans, err := query.AnswerQuestion(conn, "Quantanium 在哪卖", "brief", "")
	if err != nil {
		t.Fatal(err)
	}
	if ans["intent"] != "prices" {
		t.Fatalf("intent=%v", ans["intent"])
	}
	draft, _ := ans["answer_draft"].(string)
	if !strings.Contains(draft, "落后 Live") || !strings.Contains(draft, "非当前 Live 报价") {
		t.Fatalf("stale draft must warn about lagging snapshot: %s", draft)
	}
	if strings.Contains(draft, "当前最佳") || strings.Contains(draft, "现行") {
		t.Fatalf("stale draft must not claim current best: %s", draft)
	}
	if !strings.Contains(draft, "快照中较优") {
		t.Fatalf("stale draft should say snapshot-best: %s", draft)
	}
	ev, _ := ans["evidence"].(map[string]any)
	meta, _ := ev["meta"].(map[string]any)
	if meta["stale"] != true {
		t.Fatalf("evidence.meta.stale must be true: %#v", meta)
	}
}

func TestPriceAnswerFreshSnapshotNotMarkedStale(t *testing.T) {
	conn := staleTestDB(t, "4.10.3")
	ans, err := query.AnswerQuestion(conn, "Quantanium 在哪卖", "brief", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := ans["answer_draft"].(string)
	if strings.Contains(draft, "落后 Live") {
		t.Fatalf("fresh snapshot must not be flagged: %s", draft)
	}
	ev, _ := ans["evidence"].(map[string]any)
	if meta, ok := ev["meta"].(map[string]any); ok && meta["stale"] == true {
		t.Fatalf("fresh snapshot marked stale: %#v", meta)
	}
}
