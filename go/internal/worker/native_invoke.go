package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/frankxiong/sc-search/internal/bridge"
	"github.com/frankxiong/sc-search/internal/query"
)

// NewNativeInvoker returns an Invoker that prefers Go query tools and bridges the rest.
func NewNativeInvoker(db *sql.DB, caller *bridge.Caller) Invoker {
	return func(ctx context.Context, tool string, args map[string]any) map[string]any {
		if db != nil {
			if env, ok := tryNative(db, tool, args); ok {
				return env
			}
		}
		if caller == nil {
			return map[string]any{"ok": false, "error": fmt.Sprintf("no backend for %s", tool), "data": nil, "meta": map[string]any{}}
		}
		raw, err := caller.Call(ctx, tool, args)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error(), "data": nil, "meta": map[string]any{}}
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return map[string]any{"ok": false, "error": "bad bridge json", "data": nil, "meta": map[string]any{}}
		}
		return parsed
	}
}

func tryNative(db *sql.DB, tool string, args map[string]any) (map[string]any, bool) {
	switch tool {
	case "sc_resolve_name":
		text, _ := args["text"].(string)
		data, err := query.ResolveName(db, text)
		if err != nil {
			return errEnv(err), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_best_prices":
		commodity, _ := args["commodity"].(string)
		side, _ := args["side"].(string)
		if side == "" {
			side = "sell"
		}
		system, _ := args["system"].(string)
		limit := argInt(args["limit"], 10)
		data, err := query.BestPrices(db, commodity, side, system, limit)
		if err != nil {
			return errEnv(err), true
		}
		if len(data) == 0 {
			return noMatchEnv(fmt.Sprintf("no prices for commodity=%q side=%q", commodity, side), []any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_terminal_prices":
		terminal, _ := args["terminal"].(string)
		side, _ := args["side"].(string)
		data, err := query.TerminalPrices(db, terminal, side)
		if err != nil {
			return errEnv(err), true
		}
		if len(data) == 0 {
			return noMatchEnv(fmt.Sprintf("no prices for terminal=%q", terminal), []any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_data_freshness":
		source, _ := args["source"].(string)
		data, err := query.DataFreshness(db, source)
		if err != nil {
			return errEnv(err), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_entity_facts":
		name, _ := args["name_or_id"].(string)
		detail, _ := args["detail"].(string)
		if detail == "" {
			detail = "full"
		}
		data, err := query.EntityFacts(db, name, detail)
		if err != nil {
			return errEnv(err), true
		}
		if data == nil {
			return noMatchEnv(fmt.Sprintf("entity not found: %q", name), map[string]any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_list_missions":
		faction, _ := args["faction"].(string)
		rank, _ := args["rank_keyword"].(string)
		system, _ := args["system"].(string)
		limit := argInt(args["limit"], 10)
		data, err := query.ListMissions(db, faction, rank, system, limit)
		if err != nil {
			return errEnv(err), true
		}
		if len(data) == 0 {
			return noMatchEnv("no missions matched", []any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_locations":
		q, _ := args["query"].(string)
		system, _ := args["system"].(string)
		locType, _ := args["location_type"].(string)
		limit := argInt(args["limit"], 20)
		data, err := query.ListLocations(db, q, system, locType, limit)
		if err != nil {
			return errEnv(err), true
		}
		if len(data) == 0 {
			return noMatchEnv("no locations matched", []any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_search_docs":
		q, _ := args["query"].(string)
		limit := argInt(args["limit"], 5)
		data, err := query.SearchDocs(db, q, limit)
		if err != nil {
			return errEnv(err), true
		}
		if len(data) == 0 {
			return noMatchEnv(fmt.Sprintf("no documents matched query=%q", q), []any{}), true
		}
		return okEnv(data, map[string]any{}), true
	case "sc_intents":
		data, err := query.IntentsCatalog()
		if err != nil {
			return errEnv(err), true
		}
		return okEnv(data, map[string]any{}), true
	default:
		return nil, false
	}
}

func okEnv(data any, meta map[string]any) map[string]any {
	if meta == nil {
		meta = map[string]any{}
	}
	return map[string]any{"ok": true, "data": data, "meta": meta}
}

func errEnv(err error) map[string]any {
	return map[string]any{"ok": false, "error": err.Error(), "data": nil, "meta": map[string]any{}}
}

func noMatchEnv(message string, data any) map[string]any {
	return map[string]any{
		"ok": true, "data": data,
		"meta": map[string]any{"why": "no_match", "message": message},
	}
}

func argInt(v any, fallback int) int {
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
