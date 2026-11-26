package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frankxiong/sc-search/internal/orchestrate"
	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/router"
	"github.com/frankxiong/sc-search/internal/worker"
)

func registerOrchestrationTools(server *mcp.Server, d serverDeps) {
	registerAskTool(server, d)
	registerRouteTool(server, d)
	registerWorkerRunTool(server, d)
	registerAnswerTool(server, d)
	registerQueryPlanTool(server, d)
}

func registerAskTool(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_ask",
		Description: "Default one-shot Star Citizen entry: classify, route, execute local evidence tools, retry once when safe, and return a Chinese draft plus browser gate. Chitchat/greetings return status=not_fact with no draft: reply conversationally, do not cite the DB.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AskIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		slots := map[string]any{}
		if input.Slots != nil {
			slots = input.Slots
		} else if strings.TrimSpace(input.SlotsJSON) != "" {
			if err := json.Unmarshal([]byte(input.SlotsJSON), &slots); err != nil {
				return d.envResult(errEnv("slots_json must be a JSON object"), true)
			}
		}
		detail := input.DetailLevel
		if detail == "" {
			detail = "brief"
		}
		trace := input.TraceLevel
		if trace == "" {
			trace = "compact"
		}
		res := orchestrate.Ask(ctx, d.db, orchestrate.AskInput{
			Question:    input.Question,
			Primary:     input.Primary,
			Slots:       slots,
			DetailLevel: detail,
			TraceLevel:  trace,
		})
		body := map[string]any{
			"ok":            res.OK,
			"status":        res.Status,
			"primary":       res.Primary,
			"slots":         res.Slots,
			"path":          res.Path,
			"answer_draft":  res.AnswerDraft,
			"answer_blocks": res.AnswerBlocks,
			"evidence":      res.Evidence,
			"meta":          res.Meta,
			"browser":       res.Browser,
			"follow_ups":    res.FollowUps,
			"trace":         res.Trace,
		}
		if res.Error != nil {
			body["error"] = res.Error
		}
		return d.envResult(okEnv(body, map[string]any{"default_entry": "sc_ask"}), !res.OK && res.Status == "failed")
	})
}

func registerRouteTool(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_route",
		Description: "Expand primary+slots into a parallel DAG plan (no DB fetch).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input RouteIn) (*mcp.CallToolResult, map[string]any, error) {
		slots := map[string]any{}
		if input.Slots != nil {
			slots = input.Slots
		} else if strings.TrimSpace(input.SlotsJSON) != "" {
			if err := json.Unmarshal([]byte(input.SlotsJSON), &slots); err != nil {
				return d.envResult(errEnv("slots_json must be a JSON object"), true)
			}
		}
		replan := input.ReplanCount
		requested := strings.TrimSpace(input.Primary)
		plan, err := router.ExpandDAG(input.Question, input.Primary, slots, replan)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
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
		return d.envResult(okEnv(body, meta), false)
	})
}

func registerWorkerRunTool(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_worker_run",
		Description: "Execute a sc_route DAG in parallel waves; may augment; signals replan on evidence mismatch.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input WorkerRunIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		var plan map[string]any
		switch {
		case input.Plan != nil:
			plan = input.Plan
		case strings.TrimSpace(input.PlanJSON) != "":
			if err := json.Unmarshal([]byte(input.PlanJSON), &plan); err != nil {
				return d.envResult(errEnv("plan_json must be a JSON object"), true)
			}
		default:
			return d.envResult(errEnv("plan or plan_json is required"), true)
		}
		invoker := worker.NewNativeInvoker(d.db)
		data := worker.RunPlan(ctx, plan, invoker)
		return d.envResult(okEnv(data, map[string]any{}), false)
	})
}

func registerAnswerTool(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_answer",
		Description: "One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape). Complex multi-slot questions set route_required. Prefer sc_ask for default Q&A.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input AnswerIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		detail := input.DetailLevel
		if detail == "" {
			detail = "brief"
		}
		data, err := query.AnswerQuestion(d.db, input.Question, detail, input.HintEntity)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		return d.envResult(okEnv(data, map[string]any{}), false)
	})
}

func registerQueryPlanTool(server *mcp.Server, d serverDeps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sc_query_plan",
		Description: "Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input QueryPlanIn) (*mcp.CallToolResult, map[string]any, error) {
		if d.db == nil {
			return d.requireDB()
		}
		data, err := query.QueryPlan(d.db, input.Question, input.HintEntity)
		if err != nil {
			return d.envResult(errEnv(err.Error()), true)
		}
		return d.envResult(okEnv(data, map[string]any{}), false)
	})
}
