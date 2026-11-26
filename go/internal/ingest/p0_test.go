package ingest

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	return conn
}

func uexPayload(version string, commodityPrices map[string]float64) UexPayload {
	p := UexPayload{
		GameVersion: version,
		Terminals:   []map[string]any{{"id": 1.0, "name": "Area18", "code": "A18"}},
	}
	// Stable commodity ids: map iteration order varies, and reusing id 10 for a
	// different commodity in the next snapshot would collide on the uuid index.
	ids := map[string]float64{"Quantanium": 10, "Waste": 11}
	next := 12.0
	for _, name := range []string{"Quantanium", "Waste"} {
		sell, ok := commodityPrices[name]
		if !ok {
			continue
		}
		p.Commodities = append(p.Commodities, map[string]any{"id": ids[name], "name": name})
		p.Prices = append(p.Prices, map[string]any{"id_commodity": ids[name], "id_terminal": 1.0, "price_sell": sell})
	}
	for name, sell := range commodityPrices {
		if _, known := ids[name]; known {
			continue
		}
		id := next
		next++
		p.Commodities = append(p.Commodities, map[string]any{"id": id, "name": name})
		p.Prices = append(p.Prices, map[string]any{"id_commodity": id, "id_terminal": 1.0, "price_sell": sell})
	}
	return p
}

func priceCount(t *testing.T, conn *sql.DB, where string, args ...any) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM prices WHERE `+where, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInitDBCreatesP0IndexesIdempotent(t *testing.T) {
	conn := openTestDB(t)
	if err := InitDB(conn); err != nil {
		t.Fatalf("second InitDB: %v", err)
	}
	want := []string{
		"idx_facts_entity_key",
		"idx_relations_from_type_to",
		"idx_relations_to_type_from",
		"idx_aliases_alias_nocase",
		"idx_entities_canonical_nocase",
		"idx_prices_best",
	}
	for _, name := range want {
		var got string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&got)
		if err != nil {
			t.Fatalf("index %s missing: %v", name, err)
		}
	}
}

func TestHotPathQueriesUseIndexes(t *testing.T) {
	conn := openTestDB(t)
	cases := []struct {
		name, sql, index string
		args             []any
	}{
		{"facts by entity", `SELECT key FROM facts WHERE entity_id = ?`, "idx_facts_entity_key", []any{1}},
		{"relations from", `SELECT to_id FROM relations WHERE from_id = ? AND rel_type = 'x'`, "idx_relations_from_type_to", []any{1}},
		{"relations to", `SELECT from_id FROM relations WHERE to_id = ? AND rel_type = 'x'`, "idx_relations_to_type_from", []any{1}},
		{"alias nocase", `SELECT entity_id FROM aliases WHERE alias = ? COLLATE NOCASE`, "idx_aliases_alias_nocase", []any{"x"}},
		{"canonical nocase", `SELECT id FROM entities WHERE canonical_name = ? COLLATE NOCASE`, "idx_entities_canonical_nocase", []any{"x"}},
	}
	for _, tc := range cases {
		rows, err := conn.Query(`EXPLAIN QUERY PLAN `+tc.sql, tc.args...)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		plan := ""
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		rows.Close()
		if !strings.Contains(plan, tc.index) {
			t.Errorf("%s: plan does not use %s:\n%s", tc.name, tc.index, plan)
		}
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	conn := openTestDB(t)
	boom := errors.New("boom")
	err := WithTx(conn, func(tx *sql.Tx) error {
		if _, err := UpsertEntity(tx, "ship", "Rollback Ship", "t", "", ""); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM entities WHERE canonical_name = 'Rollback Ship'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("entity leaked after rollback: %d", n)
	}
}

func TestUexSnapshotRemovesDelistedPrices(t *testing.T) {
	conn := openTestDB(t)
	if _, err := SyncUexPayload(conn, uexPayload("4.0-test", map[string]float64{"Quantanium": 80, "Waste": 5})); err != nil {
		t.Fatal(err)
	}
	if n := priceCount(t, conn, `source = 'uex'`); n != 2 {
		t.Fatalf("after A: %d prices", n)
	}
	// Second snapshot no longer lists Waste.
	if _, err := SyncUexPayload(conn, uexPayload("4.0-test", map[string]float64{"Quantanium": 90})); err != nil {
		t.Fatal(err)
	}
	if n := priceCount(t, conn, `source = 'uex'`); n != 1 {
		t.Fatalf("after B: want 1 price, got %d", n)
	}
	var p float64
	if err := conn.QueryRow(`SELECT price FROM prices WHERE source = 'uex'`).Scan(&p); err != nil || p != 90 {
		t.Fatalf("price=%v err=%v", p, err)
	}
}

func TestUexEmptyPricesDoNotWipeSnapshot(t *testing.T) {
	conn := openTestDB(t)
	if _, err := SyncUexPayload(conn, uexPayload("4.0-test", map[string]float64{"Quantanium": 80})); err != nil {
		t.Fatal(err)
	}
	empty := uexPayload("4.0-test", nil)
	if _, err := SyncUexPayload(conn, empty); err != nil {
		t.Fatal(err)
	}
	if n := priceCount(t, conn, `source = 'uex'`); n != 1 {
		t.Fatalf("empty reply must keep old snapshot, got %d", n)
	}
}

func TestUexKeepsOtherVersionsByDefault(t *testing.T) {
	conn := openTestDB(t)
	if _, err := SyncUexPayload(conn, uexPayload("3.24.2", map[string]float64{"Quantanium": 80})); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncUexPayload(conn, uexPayload("4.10.3", map[string]float64{"Quantanium": 95})); err != nil {
		t.Fatal(err)
	}
	if n := priceCount(t, conn, `source = 'uex'`); n != 2 {
		t.Fatalf("both versions expected by default, got %d", n)
	}
	t.Setenv("SC_SEARCH_UEX_PRUNE_OTHER_VERSIONS", "1")
	if _, err := SyncUexPayload(conn, uexPayload("4.10.3", map[string]float64{"Quantanium": 96})); err != nil {
		t.Fatal(err)
	}
	if n := priceCount(t, conn, `source = 'uex' AND game_version != '4.10.3'`); n != 0 {
		t.Fatalf("old version should be pruned with env, got %d", n)
	}
}

func TestUexFailureMidSyncKeepsPreviousSnapshot(t *testing.T) {
	conn := openTestDB(t)
	if _, err := SyncUexPayload(conn, uexPayload("4.0-test", map[string]float64{"Quantanium": 80, "Waste": 5})); err != nil {
		t.Fatal(err)
	}
	runID, err := StartRun(conn, uexSource, "")
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("simulated failure after apply")
	_, err = applyInTx(conn, func(tx *sql.Tx) (int, error) {
		if _, err := ApplyUex(tx, runID, uexPayload("4.0-test", map[string]float64{"Quantanium": 1})); err != nil {
			return 0, err
		}
		return 0, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if n := priceCount(t, conn, `source = 'uex'`); n != 2 {
		t.Fatalf("previous snapshot must survive rollback, got %d prices", n)
	}
	var p float64
	if err := conn.QueryRow(`
		SELECT p.price FROM prices p JOIN entities e ON e.id = p.commodity_entity_id
		WHERE e.canonical_name = 'Quantanium'
	`).Scan(&p); err != nil || p != 80 {
		t.Fatalf("price=%v err=%v", p, err)
	}
}

func TestSyncScmdbFailureKeepsPreviousFacts(t *testing.T) {
	conn := openTestDB(t)
	payload := map[string]any{
		"version": "4.0-test",
		"factions": map[string]any{
			"f1": map[string]any{"name": "Test Faction"},
		},
	}
	if _, err := SyncScmdbPayload(conn, payload); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM entities WHERE kind = 'faction'`).Scan(&before); err != nil || before == 0 {
		t.Fatalf("before=%d err=%v", before, err)
	}
	// Plant a fact that a successful re-sync would clear; a failed apply must keep it.
	var eid int64
	if err := conn.QueryRow(`SELECT id FROM entities WHERE kind = 'faction'`).Scan(&eid); err != nil {
		t.Fatal(err)
	}
	run := int64(1)
	if err := UpsertFact(conn, eid, "marker", scmdbSource, "4.0-test", &run, "keep-me", nil, ""); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	_, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		if _, err := ApplyScmdbPayload(tx, payload, 2, "4.0-test"); err != nil {
			return 0, err
		}
		return 0, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	var v string
	if err := conn.QueryRow(`SELECT value_text FROM facts WHERE key = 'marker'`).Scan(&v); err != nil || v != "keep-me" {
		t.Fatalf("clear leaked despite rollback: v=%q err=%v", v, err)
	}
}
