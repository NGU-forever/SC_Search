package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
)

func registerPriceTools(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_best_prices",
		Description: "Best buy/sell prices for a commodity across terminals (local UEX snapshot).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BestPricesIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		side := input.Side
		if side == "" {
			side = "sell"
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 10
		}
		data, err := query.BestPrices(d.db, input.Commodity, side, input.System, limit)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return d.envResult(noMatchEnv(fmt.Sprintf("no prices for commodity=%q side=%q", input.Commodity, side), nil), false)
		}
		meta := d.withFreshness(data)
		meta["source"] = "uex"
		return d.envResult(okEnv(data, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_terminal_prices",
		Description: "List commodity prices at a terminal from the local UEX snapshot.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TerminalPricesIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		data, err := query.TerminalPrices(d.db, input.Terminal, input.Side)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return d.envResult(noMatchEnv(fmt.Sprintf("no prices for terminal=%q", input.Terminal), nil), false)
		}
		meta := d.withFreshness(data)
		meta["source"] = "uex"
		return d.envResult(okEnv(data, meta), false)
	})
}
