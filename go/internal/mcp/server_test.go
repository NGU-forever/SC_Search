package mcp

import (
	"context"
	"database/sql"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

var allToolNames = []string{
	"sc_resolve_name",
	"sc_entity_facts",
	"sc_search_docs",
	"sc_best_prices",
	"sc_terminal_prices",
	"sc_list_missions",
	"sc_locations",
	"sc_data_freshness",
	"sc_intents",
	"sc_ask",
	"sc_route",
	"sc_worker_run",
	"sc_answer",
	"sc_query_plan",
}

// dbToolNames are the tools that require a database handle.
var dbToolNames = []string{
	"sc_resolve_name",
	"sc_entity_facts",
	"sc_search_docs",
	"sc_best_prices",
	"sc_terminal_prices",
	"sc_list_missions",
	"sc_locations",
	"sc_data_freshness",
	"sc_ask",
	"sc_worker_run",
	"sc_answer",
	"sc_query_plan",
}

func connectServer(t *testing.T, db *sql.DB) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := NewServer(db)
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "sc-search-test", Version: "0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func callTool(t *testing.T, session *sdk.ClientSession, name string, args any) (*sdk.CallToolResult, map[string]any) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: CallTool: %v", name, err)
	}
	env, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("%s: StructuredContent type %T, want map[string]any", name, res.StructuredContent)
	}
	return res, env
}

// toolArgs returns a valid input for each tool. sc_worker_run is derived from
// sc_route's plan so the contract test exercises both tools end to end.
func toolArgs(t *testing.T, session *sdk.ClientSession) map[string]any {
	t.Helper()
	_, routeEnv := callTool(t, session, "sc_route", map[string]any{
		"question": "卡拉克货舱多大",
		"primary":  "ship_stats",
	})
	plan, _ := routeEnv["data"].(map[string]any)
	if plan == nil {
		t.Fatal("sc_route returned no plan data")
	}
	return map[string]any{
		"sc_resolve_name":    map[string]any{"text": "卡拉克"},
		"sc_entity_facts":    map[string]any{"name_or_id": "Anvil Carrack"},
		"sc_search_docs":     map[string]any{"query": "Quantanium"},
		"sc_best_prices":     map[string]any{"commodity": "黄金", "side": "sell"},
		"sc_terminal_prices": map[string]any{"terminal": "Area 18 TDD"},
		"sc_list_missions":   map[string]any{"faction": "Bit Zeros", "limit": 5},
		"sc_locations":       map[string]any{"limit": 5},
		"sc_data_freshness":  map[string]any{},
		"sc_intents":         map[string]any{},
		"sc_ask":             map[string]any{"question": "卡拉克货舱多大"},
		"sc_route":           map[string]any{"question": "卡拉克货舱多大", "primary": "ship_stats"},
		"sc_worker_run":      map[string]any{"plan": plan},
		"sc_answer":          map[string]any{"question": "卡拉克货舱多大"},
		"sc_query_plan":      map[string]any{"question": "卡拉克货舱多大"},
	}
}

func TestServerListsAllTools(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	session := connectServer(t, testfixture.New(t))
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	if len(got) != len(allToolNames) {
		t.Fatalf("listed %d tools, want %d: %v", len(got), len(allToolNames), got)
	}
	for _, name := range allToolNames {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestServerToolEnvelope(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	engines := []struct {
		name string
		opts testfixture.Options
	}{
		{name: "fresh", opts: testfixture.Options{UEXGameVersion: testfixture.FreshUEXVersion}},
		{name: "stale", opts: testfixture.Options{UEXGameVersion: testfixture.StaleUEXVersion}},
	}
	for _, eng := range engines {
		eng := eng
		t.Run(eng.name, func(t *testing.T) {
			session := connectServer(t, testfixture.NewWithOptions(t, eng.opts))
			args := toolArgs(t, session)
			for _, name := range allToolNames {
				name := name
				t.Run(name, func(t *testing.T) {
					res, env := callTool(t, session, name, args[name])
					if res.IsError {
						t.Fatalf("IsError=true env=%v", env)
					}
					assertEnvelope(t, env)
					if env["ok"] != true {
						t.Fatalf("ok=%v env=%v", env["ok"], env)
					}
				})
			}
		})
	}
}

func TestServerPriceStaleMeta(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	session := connectServer(t, testfixture.NewWithOptions(t, testfixture.Options{
		UEXGameVersion: testfixture.StaleUEXVersion,
	}))
	res, env := callTool(t, session, "sc_best_prices", map[string]any{"commodity": "黄金", "side": "sell"})
	if res.IsError {
		t.Fatalf("IsError=true env=%v", env)
	}
	assertEnvelope(t, env)
	meta, _ := env["meta"].(map[string]any)
	if meta["stale"] != true {
		t.Fatalf("stale meta=%v, want true", meta)
	}
	if meta["source"] != "uex" {
		t.Fatalf("meta.source=%v, want uex", meta["source"])
	}
}

func TestServerNilDB(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	session := connectServer(t, nil)
	args := map[string]any{
		"sc_resolve_name":    map[string]any{"text": "卡拉克"},
		"sc_entity_facts":    map[string]any{"name_or_id": "Anvil Carrack"},
		"sc_search_docs":     map[string]any{"query": "Quantanium"},
		"sc_best_prices":     map[string]any{"commodity": "黄金"},
		"sc_terminal_prices": map[string]any{"terminal": "Area 18 TDD"},
		"sc_list_missions":   map[string]any{},
		"sc_locations":       map[string]any{},
		"sc_data_freshness":  map[string]any{},
		"sc_ask":             map[string]any{"question": "卡拉克货舱多大"},
		"sc_worker_run":      map[string]any{"plan": map[string]any{"primary": "other", "question": "x", "calls": []any{}}},
		"sc_answer":          map[string]any{"question": "卡拉克货舱多大"},
		"sc_query_plan":      map[string]any{"question": "卡拉克货舱多大"},
	}
	for _, name := range dbToolNames {
		name := name
		t.Run(name, func(t *testing.T) {
			res, env := callTool(t, session, name, args[name])
			if !res.IsError {
				t.Fatalf("IsError=false env=%v", env)
			}
			assertEnvelope(t, env)
			if env["ok"] != false {
				t.Fatalf("ok=%v env=%v", env["ok"], env)
			}
			if msg, _ := env["error"].(string); msg == "" {
				t.Fatalf("missing error message: %v", env)
			}
		})
	}
}

func assertEnvelope(t *testing.T, env map[string]any) {
	t.Helper()
	if _, ok := env["ok"]; !ok {
		t.Fatalf("envelope missing ok: %v", env)
	}
	if _, ok := env["data"]; !ok {
		t.Fatalf("envelope missing data: %v", env)
	}
	if _, ok := env["meta"].(map[string]any); !ok {
		t.Fatalf("envelope missing meta object: %v", env)
	}
}
