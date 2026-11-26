package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
)

func registerEntityTools(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_resolve_name",
		Description: "Resolve Star Citizen Chinese/English/slang names to entities in the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveNameIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		data, err := query.ResolveName(d.db, input.Text)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		return d.envResult(okEnv(data, map[string]any{}), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_entity_facts",
		Description: "Facts and relations for a mission, blueprint, ship, item, or faction from local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EntityFactsIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		detail := input.Detail
		if detail == "" {
			detail = "compact"
		}
		data, err := query.EntityFacts(d.db, input.NameOrID, detail)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if data == nil {
			return d.envResult(noMatchEnv(fmt.Sprintf("entity not found: %q", input.NameOrID), map[string]any{}), false)
		}
		meta := map[string]any{}
		if ent, ok := data["entity"].(map[string]any); ok {
			meta["game_version"] = ent["game_version"]
		}
		return d.envResult(okEnv(data, query.AttachFreshness(d.db, meta)), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_search_docs",
		Description: "Search local Wiki/doc snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchDocsIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 5
		}
		data, err := query.SearchDocs(d.db, input.Query, limit)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return d.envResult(noMatchEnv(fmt.Sprintf("no documents matched query=%q", input.Query), nil), false)
		}
		return d.envResult(okEnv(data, d.withFreshness(data)), false)
	})
}
