package ingest

import (
	"path/filepath"
	"testing"
)

func TestParseINI(t *testing.T) {
	m := ParseINI("vehicle_Name_Gladius=Aegis Gladius\n;comment\nvehicle_Name_Gladius_zh=短剑\n")
	if m["vehicle_Name_Gladius"] != "Aegis Gladius" {
		t.Fatalf("%#v", m)
	}
}

func TestApplyLocalizationMaps(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	en := map[string]string{"vehicle_Name_Gladius": "Aegis Gladius"}
	zh := map[string]string{"vehicle_Name_Gladius": "短剑"}
	n, err := SyncLocalization(conn, dir, en, zh)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("count=%d", n)
	}
	var alias string
	err = conn.QueryRow(`
		SELECT a.alias FROM aliases a
		JOIN entities e ON e.id = a.entity_id
		WHERE a.lang = 'zh' AND e.canonical_name = 'Aegis Gladius'
	`).Scan(&alias)
	if err != nil || alias != "短剑" {
		t.Fatalf("alias=%q err=%v", alias, err)
	}
}
