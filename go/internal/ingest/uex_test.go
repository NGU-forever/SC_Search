package ingest

import (
	"path/filepath"
	"testing"
)

func TestSyncUexPayload(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	n, err := SyncUexPayload(conn, UexPayload{
		GameVersion: "4.0-test",
		Terminals: []map[string]any{
			{"id": 1.0, "name": "Area18", "code": "A18"},
		},
		Commodities: []map[string]any{
			{"id": 10.0, "name": "Quantanium", "slug": "quantanium"},
		},
		Prices: []map[string]any{
			{"id_commodity": 10.0, "id_terminal": 1.0, "price_buy": 50.0, "price_sell": 80.0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Fatalf("row_count=%d", n)
	}
	var price float64
	err = conn.QueryRow(`
		SELECT price FROM prices WHERE side = 'sell' AND source = 'uex'
	`).Scan(&price)
	if err != nil || price != 80 {
		t.Fatalf("price=%v err=%v", price, err)
	}
}
