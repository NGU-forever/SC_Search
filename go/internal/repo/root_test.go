package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindRoot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	root, err := FindRoot(filepath.Dir(thisFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go", "go.mod")); err != nil {
		t.Fatalf("expected go/go.mod under %s: %v", root, err)
	}
}

func TestFindRootReleaseMarker(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, releaseMarker)
	if err := os.WriteFile(marker, []byte("release\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := FindRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if root != dir {
		t.Fatalf("got %q want %q", root, dir)
	}
}
