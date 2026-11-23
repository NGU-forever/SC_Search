// Package mcp registers the sc-search MCP tools.
// Dual-track: native Go for ported query tools; remaining tools bridge to Python.
package mcp

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/bridge"
	"github.com/frankxiong/sc-search/internal/query"
)

// NewServer builds the stdio MCP server with all tool names registered.
func NewServer(caller *bridge.Caller, db *sql.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sc-search",
		Version: "0.3.0-go-partial-native",
	}, nil)

	add := func(name, desc string, fn mcp.ToolHandlerFor[map[string]any, string]) {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: desc}, fn)
	}

	textResult := func(raw string, isErr bool) (*mcp.CallToolResult, string, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: raw}},
			IsError: isErr,
		}, raw, nil
	}

	bridgeHandler := func(tool string) mcp.ToolHandlerFor[map[string]any, string] {
		return func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if input == nil {
				input = map[string]any{}
			}
			raw, err := caller.Call(ctx, tool, input)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			return textResult(raw, false)
		}
	}

	withFreshness := func(rows []map[string]any) map[string]any {
		meta := map[string]any{}
		if len(rows) > 0 {
			meta["game_version"] = rows[0]["game_version"]
			if v, ok := rows[0]["reported_at"]; ok && v != nil {
				meta["synced_at"] = v
			} else if v, ok := rows[0]["finished_at"]; ok && v != nil {
				meta["synced_at"] = v
			}
		}
		if db == nil {
			return meta
		}
		live, _ := query.LiveGameVersion(db)
		meta["as_of"] = live
		rowVer := ""
		if gv, ok := meta["game_version"]; ok {
			rowVer = query.ShortLiveLabel(fmt.Sprint(gv))
		}
		meta["stale"] = rowVer != "" && live != "" && rowVer != live
		return meta
	}

	add("sc_resolve_name", "Resolve Star Citizen Chinese/English/slang names to entities in the local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_resolve_name")(ctx, req, input)
			}
			text, _ := input["text"].(string)
			data, err := query.ResolveName(db, text)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			return textResult(okJSON(data, map[string]any{}), false)
		})

	add("sc_best_prices", "Best buy/sell prices for a commodity across terminals (local UEX snapshot).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_best_prices")(ctx, req, input)
			}
			commodity, _ := input["commodity"].(string)
			side, _ := input["side"].(string)
			if side == "" {
				side = "sell"
			}
			system, _ := input["system"].(string)
			limit := asInt(input["limit"], 10)
			data, err := query.BestPrices(db, commodity, side, system, limit)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if len(data) == 0 {
				return textResult(noMatchJSON(fmt.Sprintf("no prices for commodity=%q side=%q", commodity, side), nil), false)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_terminal_prices", "List commodity prices at a terminal from the local UEX snapshot.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_terminal_prices")(ctx, req, input)
			}
			terminal, _ := input["terminal"].(string)
			side, _ := input["side"].(string)
			data, err := query.TerminalPrices(db, terminal, side)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if len(data) == 0 {
				return textResult(noMatchJSON(fmt.Sprintf("no prices for terminal=%q", terminal), nil), false)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_data_freshness", "Show last ingest_runs freshness (game_version and sync time) per source.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_data_freshness")(ctx, req, input)
			}
			source, _ := input["source"].(string)
			data, err := query.DataFreshness(db, source)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_entity_facts", "Facts and relations for a mission, blueprint, ship, item, or faction from local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_entity_facts")(ctx, req, input)
			}
			name, _ := input["name_or_id"].(string)
			detail, _ := input["detail"].(string)
			if detail == "" {
				detail = "full"
			}
			data, err := query.EntityFacts(db, name, detail)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if data == nil {
				return textResult(noMatchJSON(fmt.Sprintf("entity not found: %q", name), map[string]any{}), false)
			}
			meta := map[string]any{}
			if ent, ok := data["entity"].(map[string]any); ok {
				meta["game_version"] = ent["game_version"]
			}
			live, _ := query.LiveGameVersion(db)
			meta["as_of"] = live
			rowVer := query.ShortLiveLabel(fmt.Sprint(meta["game_version"]))
			meta["stale"] = rowVer != "" && live != "" && rowVer != live
			return textResult(okJSON(data, meta), false)
		})

	add("sc_locations", "List locations/terminals (optionally by system) from the local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_locations")(ctx, req, input)
			}
			q, _ := input["query"].(string)
			system, _ := input["system"].(string)
			locType, _ := input["location_type"].(string)
			limit := asInt(input["limit"], 20)
			data, err := query.ListLocations(db, q, system, locType, limit)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if len(data) == 0 {
				return textResult(noMatchJSON("no locations matched", nil), false)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_search_docs", "Search local Wiki/doc snippets.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_search_docs")(ctx, req, input)
			}
			q, _ := input["query"].(string)
			limit := asInt(input["limit"], 5)
			data, err := query.SearchDocs(db, q, limit)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if len(data) == 0 {
				return textResult(noMatchJSON(fmt.Sprintf("no documents matched query=%q", q), nil), false)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_list_missions", "List missions filtered by faction/system/type from the local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, string, error) {
			if db == nil {
				return bridgeHandler("sc_list_missions")(ctx, req, input)
			}
			faction, _ := input["faction"].(string)
			rankKeyword, _ := input["rank_keyword"].(string)
			system, _ := input["system"].(string)
			limit := asInt(input["limit"], 10)
			data, err := query.ListMissions(db, faction, rankKeyword, system, limit)
			if err != nil {
				return textResult(errJSON(err.Error()), true)
			}
			if len(data) == 0 {
				return textResult(noMatchJSON("no missions matched", nil), false)
			}
			return textResult(okJSON(data, withFreshness(data)), false)
		})

	add("sc_query_plan", "Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape).", bridgeHandler("sc_query_plan"))
	add("sc_answer", "One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape).", bridgeHandler("sc_answer"))
	add("sc_intents", "List valid Router primaries, slots, tools, and paths (machine-readable playbook).", bridgeHandler("sc_intents"))
	add("sc_route", "Expand primary+slots into a parallel DAG plan (no DB fetch).", bridgeHandler("sc_route"))
	add("sc_worker_run", "Execute a sc_route DAG in parallel waves; may augment; signals replan on evidence mismatch.", bridgeHandler("sc_worker_run"))

	return server
}

func asInt(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return fallback
	}
}
