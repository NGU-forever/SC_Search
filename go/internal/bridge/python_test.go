package bridge

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCallScIntents(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	// go/internal/bridge -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	c := New(root)
	raw, err := c.Call(context.Background(), "sc_intents", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("json: %v\nraw=%s", err, raw)
	}
	if !env.OK {
		t.Fatalf("envelope not ok: %s", raw)
	}
	if _, ok := env.Data["primaries"]; !ok {
		t.Fatalf("missing primaries: %s", raw)
	}
}

func TestFindRepoRoot(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	start := filepath.Dir(thisFile)
	root, err := FindRepoRoot(start)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != "SC_Search" && !fileExists(filepath.Join(root, "go", "go.mod")) {
		t.Fatalf("unexpected root %s", root)
	}
}
