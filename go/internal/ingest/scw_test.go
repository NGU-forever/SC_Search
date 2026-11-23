package ingest

import (
	"path/filepath"
	"testing"
)

func TestApplyScwVehicles(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	n, err := SyncScwVehicles(conn, []map[string]any{
		{
			"uuid": "u1", "name": "Gladius", "game_name": "Aegis Gladius", "slug": "gladius",
			"cargo_capacity": 0.0,
			"manufacturer":   map[string]any{"name": "Aegis Dynamics", "code": "AEGS"},
			"size":           map[string]any{"en_EN": "Small"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestApplyScwBlueprints(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
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
