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
		Version: "1.1.0-go",
	}, nil)

	// Return structured map (not JSON string) so hosts that validate structuredContent as object succeed.
	envResult := func(env map[string]any, isErr bool) (*mcp.CallToolResult, map[string]any, error) {
		return &mcp.CallToolResult{IsError: isErr}, env, nil
	}

	requireDB := func() (*mcp.CallToolResult, map[string]any, error) {
		return envResult(errEnv("database unavailable"), true)
	}

	withFreshness := func(rows []map[string]any) map[string]any {
		meta := query.MetaFromRows(rows)
		if db == nil {
			return meta
		}
		return query.AttachFreshness(db, meta)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_resolve_name",
		Description: "Resolve Star Citizen Chinese/English/slang names to entities in the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ResolveNameIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		data, err := query.ResolveName(db, input.Text)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		return envResult(okEnv(data, map[string]any{}), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_best_prices",
		Description: "Best buy/sell prices for a commodity across terminals (local UEX snapshot).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BestPricesIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		side := input.Side
		if side == "" {
			side = "sell"
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 10
		}
		data, err := query.BestPrices(db, input.Commodity, side, input.System, limit)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return envResult(noMatchEnv(fmt.Sprintf("no prices for commodity=%q side=%q", input.Commodity, side), nil), false)
		}
		meta := withFreshness(data)
		meta["source"] = "uex"
		return envResult(okEnv(data, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_terminal_prices",
		Description: "List commodity prices at a terminal from the local UEX snapshot.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input TerminalPricesIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		data, err := query.TerminalPrices(db, input.Terminal, input.Side)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return envResult(noMatchEnv(fmt.Sprintf("no prices for terminal=%q", input.Terminal), nil), false)
		}
		meta := withFreshness(data)
		meta["source"] = "uex"
		return envResult(okEnv(data, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_data_freshness",
		Description: "Show last ingest_runs freshness (game_version and sync time) per source.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input DataFreshnessIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		data, err := query.DataFreshness(db, input.Source)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		annotated, meta := query.AnnotateFreshnessRows(db, data)
		return envResult(okEnv(annotated, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_entity_facts",
		Description: "Facts and relations for a mission, blueprint, ship, item, or faction from local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EntityFactsIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		detail := input.Detail
		if detail == "" {
			detail = "compact"
		}
		data, err := query.EntityFacts(db, input.NameOrID, detail)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if data == nil {
			return envResult(noMatchEnv(fmt.Sprintf("entity not found: %q", input.NameOrID), map[string]any{}), false)
		}
		meta := map[string]any{}
		if ent, ok := data["entity"].(map[string]any); ok {
			meta["game_version"] = ent["game_version"]
		}
		return envResult(okEnv(data, query.AttachFreshness(db, meta)), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_locations",
		Description: "List locations/terminals (optionally by system) from the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input LocationsIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 20
		}
		data, err := query.ListLocations(db, input.Query, input.System, input.LocationType, limit)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return envResult(noMatchEnv("no locations matched", nil), false)
		}
		return envResult(okEnv(data, withFreshness(data)), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_search_docs",
		Description: "Search local Wiki/doc snippets.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchDocsIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 5
		}
		data, err := query.SearchDocs(db, input.Query, limit)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if len(data) == 0 {
			return envResult(noMatchEnv(fmt.Sprintf("no documents matched query=%q", input.Query), nil), false)
		}
		return envResult(okEnv(data, withFreshness(data)), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_list_missions",
		Description: "List missions filtered by faction/system/type from the local DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListMissionsIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		limit := input.Limit
		if limit <= 0 {
			limit = query.DefaultMissionLimit
		}
		list, err := query.ListMissions(db, input.Faction, input.RankKeyword, input.System, limit)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		if len(list.Rows) == 0 {
			return envResult(noMatchEnv("no missions matched", nil), false)
		}
		meta := withFreshness(list.Rows)
		meta["total"] = list.Total
		meta["truncated"] = list.Truncated
		meta["limit"] = list.Limit
		return envResult(okEnv(list.Rows, meta), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_query_plan",
		Description: "Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input QueryPlanIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		data, err := query.QueryPlan(db, input.Question, input.HintEntity)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		return envResult(okEnv(data, map[string]any{}), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_answer",
		Description: "One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape). Complex multi-slot questions set route_required.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnswerIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		detail := input.DetailLevel
		if detail == "" {
			detail = "brief"
		}
		data, err := query.AnswerQuestion(db, input.Question, detail, input.HintEntity)
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		return envResult(okEnv(data, map[string]any{}), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_intents",
		Description: "List valid Router primaries, slots, tools, and paths (machine-readable playbook).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input EmptyIn) (*mcp.CallToolResult, map[string]any, error) {
		data, err := query.IntentsCatalog()
		if err != nil {
			return envResult(errEnv(err.Error()), true)
		}
		return envResult(okEnv(data, map[string]any{}), false)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_route",
		Description: "Expand primary+slots into a parallel DAG plan (no DB fetch).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RouteIn) (*mcp.CallToolResult, map[string]any, error) {
		slots := map[string]any{}
		if input.Slots != nil {
			slots = input.Slots
		} else if strings.TrimSpace(input.SlotsJSON) != "" {
			if err := json.Unmarshal([]byte(input.SlotsJSON), &slots); err != nil {
				return envResult(errEnv("slots_json must be a JSON object"), true)
			}
		}
		replan := input.ReplanCount
		requested := strings.TrimSpace(input.Primary)
		plan, err := router.ExpandDAG(input.Question, input.Primary, slots, replan)
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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_worker_run",
		Description: "Execute a sc_route DAG in parallel waves; may augment; signals replan on evidence mismatch.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WorkerRunIn) (*mcp.CallToolResult, map[string]any, error) {
		if db == nil {
			return requireDB()
		}
		var plan map[string]any
		switch {
		case input.Plan != nil:
			plan = input.Plan
		case strings.TrimSpace(input.PlanJSON) != "":
			if err := json.Unmarshal([]byte(input.PlanJSON), &plan); err != nil {
				return envResult(errEnv("plan_json must be a JSON object"), true)
			}
		default:
			return envResult(errEnv("plan or plan_json is required"), true)
		}
		invoker := worker.NewNativeInvoker(db)
		data := worker.RunPlan(ctx, plan, invoker)
		return envResult(okEnv(data, map[string]any{}), false)
	})

	return server
}
