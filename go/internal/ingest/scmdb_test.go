package ingest

import (
	"path/filepath"
	"testing"
)

func TestSyncScmdbPayload(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	n, err := SyncScmdbPayload(conn, map[string]any{
		"version": "4.0-scmdb-test",
		"factions": map[string]any{
			"fg1": map[string]any{"name": "TestFaction"},
		},
		"locationPools": map[string]any{
			"loc1": map[string]any{"name": "RAB-1", "type": "Station", "system": "Stanton"},
		},
		"contracts": []any{
			map[string]any{
				"id":           "m1",
				"debugName":    "mission_test_1",
				"title":        "Test Mission",
				"rewardUEC":    1000.0,
				"factionGuid":  "fg1",
				"locations":    []any{"loc1"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Fatalf("count=%d", n)
	}
	var reward float64
	err = conn.QueryRow(`
		SELECT value_num FROM facts WHERE key = 'reward_uec' AND source = 'scmdb'
	`).Scan(&reward)
	if err != nil || reward != 1000 {
		t.Fatalf("reward=%v err=%v", reward, err)
	}
}
