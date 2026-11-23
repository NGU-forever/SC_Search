package ingest

import (
	"path/filepath"
	"testing"
)

func TestWikiTitleCandidates(t *testing.T) {
	got := WikiTitleCandidates("Aegis Gladius")
	if len(got) < 2 || got[0] != "Aegis Gladius" || got[1] != "Gladius" {
		t.Fatalf("unexpected candidates: %#v", got)
	}
}

func TestSyncWikiDocuments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.sqlite")
	conn, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	n, err := SyncWikiDocuments(conn, []WikiDoc{
		{Title: "Gladius", Body: "light fighter", URL: "https://starcitizen.tools/Gladius"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d", n)
	}
	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM documents WHERE source = ?`, wikiSource).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("documents=%d", count)
	}
}
