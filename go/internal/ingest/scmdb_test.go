package ingest

import (
	"encoding/json"
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

func TestMissionSitePoolByRegion(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	_, err = SyncScmdbPayload(conn, map[string]any{
		"version": "4.0-scmdb-test",
		"locationPools": map[string]any{
			"a1": map[string]any{"name": "Checkmate", "type": "Station"},
			"b1": map[string]any{"name": "Orbituary", "type": "Station"},
			"c1": map[string]any{"name": "Ruin Station", "type": "Station"},
			"d1": map[string]any{"name": "Gaslight", "type": "Outpost"},
		},
		"contracts": []any{
			map[string]any{
				"id":                     "heist1",
				"debugName":              "UnafilliatedCFP_Pyro_RegionA_M_2steroidBase_Criminals_ShipHeist",
				"title":                  "Ship Heist",
				"personalCooldownTime":   45.0,
				"timeToComplete":         1.0,
				"buyIn":                  33000.0,
				"pyroRegion":             []any{"A", "B", "C", "D"},
				"locationSets": []any{
					map[string]any{"locations": []any{"a1"}},
					map[string]any{"locations": []any{"b1"}},
					map[string]any{"locations": []any{"c1"}},
					map[string]any{"locations": []any{"d1"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var byRegionJSON, cooldownUnit, band string
	var cooldown float64
	var cooldownU string
	err = conn.QueryRow(`
		SELECT
		  (SELECT value_text FROM facts WHERE key='site_pool_by_region' LIMIT 1),
		  (SELECT value_text FROM facts WHERE key='cooldown_unit' LIMIT 1),
		  (SELECT value_text FROM facts WHERE key='difficulty_band' LIMIT 1),
		  (SELECT value_num FROM facts WHERE key='cooldown' LIMIT 1),
		  (SELECT unit FROM facts WHERE key='cooldown' LIMIT 1)
	`).Scan(&byRegionJSON, &cooldownUnit, &band, &cooldown, &cooldownU)
	if err != nil {
		t.Fatal(err)
	}
	if cooldownUnit != "min" || cooldownU != "min" || cooldown != 45 {
		t.Fatalf("cooldown meta unit=%q fact_unit=%q num=%v", cooldownUnit, cooldownU, cooldown)
	}
	if band != "M" {
		t.Fatalf("difficulty_band=%q", band)
	}
	var byRegion map[string][]string
	if err := json.Unmarshal([]byte(byRegionJSON), &byRegion); err != nil {
		t.Fatal(err)
	}
	if byRegion["A"][0] != "Checkmate" || byRegion["B"][0] != "Orbituary" {
		t.Fatalf("byRegion=%#v", byRegion)
	}
}
