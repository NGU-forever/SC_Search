package query

import (
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

// ClassificationContext carries intent, question, and entities for Primary selection.
type ClassificationContext struct {
	Intent   string
	Question string
	Entities []map[string]any
}

// IntentToSuggestedPrimary maps intents without entity/question context (compat).
func IntentToSuggestedPrimary(intent string) string {
	return SuggestedPrimary(ClassificationContext{Intent: intent})
}

// SuggestedPrimary maps QueryPlan intents onto Router primaries with context.
func SuggestedPrimary(ctx ClassificationContext) string {
	intent := strings.TrimSpace(ctx.Intent)
	q := strings.TrimSpace(ctx.Question)
	switch intent {
	case "mission_list":
		if MissionRepContext(q) {
			return "mission_rep"
		}
		return "mission_money"
	case "mission_money":
		return "mission_money"
	case "prices", "trade_route":
		return "trade"
	case "loadout_calc":
		return "loadout"
	case "mining_calc":
		return "mining"
	case "earn_meta":
		return "earn_meta"
	case "howto", "client_help", "pledge_meta", "event_guide":
		return intent
	case "docs":
		return "lore_docs"
	case "facts":
		if len(ctx.Entities) > 0 && anyx.String(ctx.Entities[0]["kind"]) == "ship" {
			return "ship_stats"
		}
		return "other"
	case "ship_shop":
		return "ship_buy"
	case "live_web":
		return "patch_live"
	case "live_local", "data_sync":
		return "other"
	default:
		return "other"
	}
}

func intentToSuggestedPrimary(intent string) string {
	return IntentToSuggestedPrimary(intent)
}
