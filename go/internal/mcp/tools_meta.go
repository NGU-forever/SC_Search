package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
)

func registerMetaTools(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_data_freshness",
		Description: "Show last ingest_runs freshness (game_version and sync time) per source.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DataFreshnessIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		data, err := query.DataFreshness(d.db, input.Source)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		annotated, meta := query.AnnotateFreshnessRows(d.db, data)
		return d.envResult(okEnv(annotated, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_intents",
		Description: "List valid Router primaries, slots, tools, and paths (machine-readable playbook).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EmptyIn) (*mcp.CallToolResult, map[string]any, error) {
		data, err := query.IntentsCatalog()
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		data["default_entry"] = "sc_ask"
		return d.envResult(okEnv(data, map[string]any{"default_entry": "sc_ask"}), false)
	})
}
