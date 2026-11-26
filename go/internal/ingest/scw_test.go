package ingest

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func newScwTestDB(t *testing.T) *sql.DB {
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

func scwVehicle(uuid, name, gameName, slug string, labels map[string][2]string, foci [][2]string) map[string]any {
	v := map[string]any{
		"uuid": uuid, "name": name, "game_name": gameName, "slug": slug,
	}
	for key, pair := range labels {
		v[key] = map[string]any{"en_EN": pair[0], "zh_CN": pair[1]}
	}
	if len(foci) > 0 {
		arr := make([]any, 0, len(foci))
		for _, pair := range foci {
			arr = append(arr, map[string]any{"en_EN": pair[0], "zh_CN": pair[1]})
		}
		v["foci"] = arr
	}
	return v
}

func scwFact(t *testing.T, conn DBTX, entityID int64, key string) string {
	t.Helper()
	var value string
	err := conn.QueryRow(`
		SELECT COALESCE(value_text, '') FROM facts
		WHERE entity_id = ? AND key = ? AND source = ?
		ORDER BY id DESC LIMIT 1
	`, entityID, key, scwAPISource).Scan(&value)
	if err != nil {
		t.Fatalf("fact %s: %v", key, err)
	}
	return value
}

func scwEntityID(t *testing.T, conn DBTX, uuid string) int64 {
	t.Helper()
	var id int64
	if err := conn.QueryRow(`SELECT id FROM entities WHERE uuid = ?`, uuid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func scwAliasCount(t *testing.T, conn DBTX, alias string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM aliases WHERE source = ? AND alias = ?
	`, scwAPISource, alias).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestApplyScwVehicles(t *testing.T) {
	conn := newScwTestDB(t)
	n, err := SyncScwVehicles(conn, []map[string]any{
		{
			"uuid": "u1", "name": "Gladius", "game_name": "Aegis Gladius", "slug": "gladius",
			"cargo_capacity": 0.0,
			"manufacturer":   map[string]any{"name": "Aegis Dynamics", "code": "AEGS"},
			"size":           map[string]any{"en_EN": "Small", "zh_CN": "小"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestScwCategoryLabelsAreFactsNotAliases(t *testing.T) {
	conn := newScwTestDB(t)
	vehicles := []map[string]any{
		scwVehicle("u1", "Gladius", "Aegis Gladius", "gladius", map[string][2]string{
			"size":              {"Small", "小"},
			"type":              {"Combat", "战斗"},
			"production_status": {"Flight Ready", "可飞行"},
			"career":            {"Combat", "打击"},
		}, [][2]string{{"Combat", "战斗"}, {"Exploration", "勘探"}}),
		scwVehicle("u2", "Freelancer", "MISC Freelancer", "freelancer", map[string][2]string{
			"size":              {"Medium", "中型"},
			"type":              {"Transport", "运输船"},
			"production_status": {"Flight Ready", "可飞行"},
			"career":            {"Transport", "运输"},
		}, [][2]string{{"Transport", "运输"}, {"Exploration", "勘探"}}),
	}
	if _, err := SyncScwVehicles(conn, vehicles); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"小", "战斗", "可飞行", "打击", "中型", "运输船", "运输", "勘探"} {
		if n := scwAliasCount(t, conn, label); n != 0 {
			t.Errorf("category label %q must not be an alias, got %d rows", label, n)
		}
	}
	id := scwEntityID(t, conn, "scw-ship-u1")
	want := map[string]string{
		"size_zh":              "小",
		"type_zh":              "战斗",
		"production_status_zh": "可飞行",
		"career_zh":            "打击",
		"foci_zh":              "战斗, 勘探",
	}
	for key, value := range want {
		if got := scwFact(t, conn, id, key); got != value {
			t.Errorf("%s=%q want %q", key, got, value)
		}
	}
	// English facts are preserved.
	if got := scwFact(t, conn, id, "size"); got != "Small" {
		t.Errorf("size=%q", got)
	}
	if got := scwFact(t, conn, id, "foci"); got != "Combat, Exploration" {
		t.Errorf("foci=%q", got)
	}
}

func TestScwLegitShipAliasesRemain(t *testing.T) {
	conn := newScwTestDB(t)
	vehicles := []map[string]any{
		scwVehicle("u1", "Gladius", "Aegis Gladius", "gladius", map[string][2]string{
			"size": {"Small", "小"},
		}, nil),
	}
	if _, err := SyncScwVehicles(conn, vehicles); err != nil {
		t.Fatal(err)
	}
	id := scwEntityID(t, conn, "scw-ship-u1")
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM aliases WHERE source = ? AND entity_id = ?`, scwAPISource, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("legit aliases=%d want 3 (name, game_name, slug)", n)
	}
	for _, alias := range []string{"Gladius", "Aegis Gladius", "gladius"} {
		if got := scwAliasCount(t, conn, alias); got != 1 {
			t.Errorf("alias %q count=%d want 1", alias, got)
		}
	}
}

func TestScwRerunClearsOldPollutedAliases(t *testing.T) {
	conn := newScwTestDB(t)
	vehicles := []map[string]any{
		scwVehicle("u1", "Gladius", "Aegis Gladius", "gladius", map[string][2]string{
			"size": {"Small", "小"},
		}, nil),
	}
	if _, err := SyncScwVehicles(conn, vehicles); err != nil {
		t.Fatal(err)
	}
	id := scwEntityID(t, conn, "scw-ship-u1")
	// Curated seed aliases belong to another source and must survive the rebuild.
	if err := UpsertAlias(conn, id, "短剑", "zh", "seed"); err != nil {
		t.Fatal(err)
	}
	// Simulate a row polluted by the pre-fix writer.
	if _, err := conn.Exec(`
		INSERT INTO aliases (entity_id, alias, lang, source) VALUES (?, '可飞行', 'zh', ?)
	`, id, scwAPISource); err != nil {
		t.Fatal(err)
	}
	if n := scwAliasCount(t, conn, "可飞行"); n != 1 {
		t.Fatalf("setup polluted alias count=%d", n)
	}
	if _, err := SyncScwVehicles(conn, vehicles); err != nil {
		t.Fatal(err)
	}
	if n := scwAliasCount(t, conn, "可飞行"); n != 0 {
		t.Fatalf("rerun must delete old polluted alias, got %d rows", n)
	}
	if n := scwAliasCount(t, conn, "Gladius"); n != 1 {
		t.Fatalf("legit alias lost after rerun: %d", n)
	}
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM aliases WHERE source = ?`, scwAPISource).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("aliases rebuilt from scratch: total=%d want 3", total)
	}
	var seedAliases int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM aliases WHERE source = 'seed'`).Scan(&seedAliases); err != nil {
		t.Fatal(err)
	}
	if seedAliases != 1 {
		t.Fatalf("seed aliases must survive SCW rebuild, got %d", seedAliases)
	}
}

func TestApplyScwBlueprints(t *testing.T) {
	conn := newScwTestDB(t)
	n, err := SyncScwBlueprints(conn, []map[string]any{
		{
			"uuid": "bp1", "output_name": "Test BP", "craft_time_seconds": 60.0,
			"ingredients": []any{
				map[string]any{"name": "Agricium", "kind": "resource", "quantity_scu": 1.5},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
	var rel int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM relations WHERE rel_type = 'blueprint_requires'`).Scan(&rel); err != nil || rel != 1 {
		t.Fatalf("rel=%d err=%v", rel, err)
	}
}
