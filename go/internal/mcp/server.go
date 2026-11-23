// Package mcp registers the sc-search MCP tools (Go-native only).
package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/router"
	"github.com/frankxiong/sc-search/internal/worker"
)

// NewServer builds the stdio MCP server. db must be non-nil.
func NewServer(db *sql.DB) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "sc-search",
		Version: "1.0.0-go",
	}, nil)

	add := func(name, desc string, fn mcp.ToolHandlerFor[map[string]any, map[string]any]) {
		mcp.AddTool(server, &mcp.Tool{Name: name, Description: desc}, fn)
	}

	// Return structured map (not JSON string) so hosts that validate structuredContent as object succeed.
	envResult := func(env map[string]any, isErr bool) (*mcp.CallToolResult, map[string]any, error) {
		return &mcp.CallToolResult{IsError: isErr}, env, nil
	}

	requireDB := func() (*mcp.CallToolResult, map[string]any, error) {
		return envResult(errEnv("database unavailable"), true)
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
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			text, _ := input["text"].(string)
			data, err := query.ResolveName(db, text)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			return envResult(okEnv(data, map[string]any{}), false)
		})

	add("sc_best_prices", "Best buy/sell prices for a commodity across terminals (local UEX snapshot).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
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
				return envResult(errEnv(err.Error()), true)
			}
			if len(data) == 0 {
				return envResult(noMatchEnv(fmt.Sprintf("no prices for commodity=%q side=%q", commodity, side), nil), false)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_terminal_prices", "List commodity prices at a terminal from the local UEX snapshot.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			terminal, _ := input["terminal"].(string)
			side, _ := input["side"].(string)
			data, err := query.TerminalPrices(db, terminal, side)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			if len(data) == 0 {
				return envResult(noMatchEnv(fmt.Sprintf("no prices for terminal=%q", terminal), nil), false)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_data_freshness", "Show last ingest_runs freshness (game_version and sync time) per source.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			source, _ := input["source"].(string)
			data, err := query.DataFreshness(db, source)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_entity_facts", "Facts and relations for a mission, blueprint, ship, item, or faction from local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			name, _ := input["name_or_id"].(string)
			detail, _ := input["detail"].(string)
			if detail == "" {
				detail = "full"
			}
			data, err := query.EntityFacts(db, name, detail)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			if data == nil {
				return envResult(noMatchEnv(fmt.Sprintf("entity not found: %q", name), map[string]any{}), false)
			}
			meta := map[string]any{}
			if ent, ok := data["entity"].(map[string]any); ok {
				meta["game_version"] = ent["game_version"]
			}
			live, _ := query.LiveGameVersion(db)
			meta["as_of"] = live
			rowVer := query.ShortLiveLabel(fmt.Sprint(meta["game_version"]))
			meta["stale"] = rowVer != "" && live != "" && rowVer != live
			return envResult(okEnv(data, meta), false)
		})

	add("sc_locations", "List locations/terminals (optionally by system) from the local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			q, _ := input["query"].(string)
			system, _ := input["system"].(string)
			locType, _ := input["location_type"].(string)
			limit := asInt(input["limit"], 20)
			data, err := query.ListLocations(db, q, system, locType, limit)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			if len(data) == 0 {
				return envResult(noMatchEnv("no locations matched", nil), false)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_search_docs", "Search local Wiki/doc snippets.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			q, _ := input["query"].(string)
			limit := asInt(input["limit"], 5)
			data, err := query.SearchDocs(db, q, limit)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			if len(data) == 0 {
				return envResult(noMatchEnv(fmt.Sprintf("no documents matched query=%q", q), nil), false)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_list_missions", "List missions filtered by faction/system/type from the local DB.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			faction, _ := input["faction"].(string)
			rankKeyword, _ := input["rank_keyword"].(string)
			system, _ := input["system"].(string)
			limit := asInt(input["limit"], 10)
			data, err := query.ListMissions(db, faction, rankKeyword, system, limit)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			if len(data) == 0 {
				return envResult(noMatchEnv("no missions matched", nil), false)
			}
			return envResult(okEnv(data, withFreshness(data)), false)
		})

	add("sc_query_plan", "Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			question, _ := input["question"].(string)
			hint, _ := input["hint_entity"].(string)
			data, err := query.QueryPlan(db, question, hint)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			return envResult(okEnv(data, map[string]any{}), false)
		})

	add("sc_answer", "One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			question, _ := input["question"].(string)
			detail, _ := input["detail_level"].(string)
			if detail == "" {
				detail = "brief"
			}
			hint, _ := input["hint_entity"].(string)
			data, err := query.AnswerQuestion(db, question, detail, hint)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			return envResult(okEnv(data, map[string]any{}), false)
		})

	add("sc_intents", "List valid Router primaries, slots, tools, and paths (machine-readable playbook).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			data, err := query.IntentsCatalog()
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			return envResult(okEnv(data, map[string]any{}), false)
		})

	add("sc_route", "Expand primary+slots into a parallel DAG plan (no DB fetch).",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			question, _ := input["question"].(string)
			primary, _ := input["primary"].(string)
			slots := map[string]any{}
			switch v := input["slots_json"].(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					if err := json.Unmarshal([]byte(v), &slots); err != nil {
						return envResult(errEnv("slots_json must be a JSON object"), true)
					}
				}
			case map[string]any:
				slots = v
			}
			replan := asInt(input["replan_count"], 0)
			requested := strings.TrimSpace(primary)
			plan, err := router.ExpandDAG(question, primary, slots, replan)
			if err != nil {
				return envResult(errEnv(err.Error()), true)
			}
			body := map[string]any{
				"primary":      plan.Primary,
				"slots":        plan.Slots,
				"question":     plan.Question,
				"calls":        plan.Calls,
				"path":         plan.Path,
				"notes":        plan.Notes,
				"replan_count": plan.ReplanCount,
				"plan_intent":  plan.PlanIntent,
			}
			cat, _ := query.IntentsCatalog()
			primaries, _ := cat["primaries"].([]any)
			meta := map[string]any{
				"primaries":         primaries,
				"primary_requested": requested,
				"primary_resolved":  plan.Primary,
			}
			if requested != "" && requested != plan.Primary {
				meta["primary_fallback"] = true
			}
			return envResult(okEnv(body, meta), false)
		})

	add("sc_worker_run", "Execute a sc_route DAG in parallel waves; may augment; signals replan on evidence mismatch.",
		func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, map[string]any, error) {
			if db == nil {
				return requireDB()
			}
			var plan map[string]any
			switch v := input["plan_json"].(type) {
			case string:
				if err := json.Unmarshal([]byte(v), &plan); err != nil {
					return envResult(errEnv("plan_json must be a JSON object"), true)
				}
			case map[string]any:
				plan = v
			default:
				return envResult(errEnv("plan_json must be a JSON object"), true)
			}
			invoker := worker.NewNativeInvoker(db)
			data := worker.RunPlan(ctx, plan, invoker)
			return envResult(okEnv(data, map[string]any{}), false)
		})

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
