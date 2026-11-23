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
