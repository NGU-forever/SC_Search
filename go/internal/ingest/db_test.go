package ingest

import (
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")
	conn, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := InitDB(conn); err != nil {
		t.Fatal(err)
	}
	runs, err := RecentRuns(conn, 5)
	if err != nil {
		t.Fatal(err)
	}
	if runs == nil {
		t.Fatal("nil runs")
	}
}
