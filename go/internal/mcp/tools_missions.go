package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
)

func registerMissionTools(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_list_missions",
		Description: "List missions filtered by faction/system/type from the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMissionsIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = query.DefaultMissionLimit
		}
		list, err := query.ListMissions(d.db, input.Faction, input.RankKeyword, input.System, limit)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if len(list.Rows) == 0 {
			return d.envResult(noMatchEnv("no missions matched", nil), false)
		}
		meta := d.withFreshness(list.Rows)
		meta["total"] = list.Total
		meta["truncated"] = list.Truncated
		meta["limit"] = list.Limit
		return d.envResult(okEnv(list.Rows, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_locations",
		Description: "List locations/terminals (optionally by system) from the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input LocationsIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		data, err := query.ListLocations(d.db, input.Query, input.System, input.LocationType, limit)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return d.envResult(noMatchEnv("no locations matched", nil), false)
		}
		return d.envResult(okEnv(data, d.withFreshness(data)), false)
	})
}
