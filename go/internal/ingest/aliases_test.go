package ingest

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestSyncAliases(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
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
	n, err := SyncAliases(conn, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Fatalf("expected seed aliases, got %d", n)
	}
}
