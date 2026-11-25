package query

import "strings"

// Action is a user-action class independent of entity resolution.
type Action string

const (
	ActionNone         Action = ""
	ActionMining       Action = "mining"
	ActionLoadout      Action = "loadout"
	ActionMissionMoney Action = "mission_money"
	ActionMissionList  Action = "mission_list"
	ActionPrice        Action = "price"
	ActionClient       Action = "client_help"
	ActionPledge       Action = "pledge_meta"
	ActionEvent        Action = "event_guide"
	ActionEarn         Action = "earn_meta"
	ActionHowto        Action = "howto"
	ActionTradeRoute   Action = "trade_route"
	ActionShipShop     Action = "ship_shop"
	ActionShipBuy      Action = "ship_buy"
	ActionLiveWeb      Action = "live_web"
	ActionLiveLocal    Action = "live_local"
	ActionDataSync     Action = "data_sync"
	ActionDocs         Action = "docs"
	ActionFacts        Action = "facts"
)

// ActionMatch is the result of DetectAction.
type ActionMatch struct {
	Action     Action
	Confidence string
	RuleID     string
}

// ActionRule is one explicitly prioritized action detector.
type ActionRule struct {
	ID       string
	Priority int
	Action   Action
	Match    func(string) bool
}

// actionRules is the single ordered source of action priority (lower Priority wins).
// Behavior must stay aligned with QueryPlan branching.
func actionRules() []ActionRule {
	return []ActionRule{
		{ID: "mining_setup", Priority: 10, Action: ActionMining, Match: isMiningSetupQuestion},
		{ID: "loadout", Priority: 20, Action: ActionLoadout, Match: func(q string) bool { return loadoutRE.MatchString(q) }},
		{ID: "mission_money", Priority: 30, Action: ActionMissionMoney, Match: func(q string) bool { return missionMoneyRE.MatchString(q) }},
		{ID: "mission_list", Priority: 40, Action: ActionMissionList, Match: func(q string) bool { return missionListRE.MatchString(q) }},
		{ID: "price", Priority: 50, Action: ActionPrice, Match: func(q string) bool { return pricesRE.MatchString(q) }},
		{ID: "client", Priority: 60, Action: ActionClient, Match: func(q string) bool { return clientRE.MatchString(q) }},
		{ID: "pledge", Priority: 70, Action: ActionPledge, Match: func(q string) bool { return pledgeRE.MatchString(q) }},
		{ID: "event", Priority: 80, Action: ActionEvent, Match: func(q string) bool { return eventRE.MatchString(q) }},
		{ID: "earn", Priority: 90, Action: ActionEarn, Match: func(q string) bool { return earnMetaRE.MatchString(q) }},
		{ID: "howto", Priority: 100, Action: ActionHowto, Match: func(q string) bool { return howtoRE.MatchString(q) }},
		{ID: "trade_route", Priority: 110, Action: ActionTradeRoute, Match: func(q string) bool { return tradeRouteRE.MatchString(q) }},
		{ID: "ship_shop", Priority: 120, Action: ActionShipShop, Match: func(q string) bool { return shipShopRE.MatchString(q) }},
		{ID: "live_web", Priority: 140, Action: ActionLiveWeb, Match: func(q string) bool { return liveRE.MatchString(q) }},
		{ID: "live_local", Priority: 150, Action: ActionLiveLocal, Match: func(q string) bool { return liveLocalRE.MatchString(q) }},
		{ID: "data_sync", Priority: 160, Action: ActionDataSync, Match: func(q string) bool { return dataSyncRE.MatchString(q) }},
		{ID: "docs", Priority: 170, Action: ActionDocs, Match: func(q string) bool { return docsRE.MatchString(q) }},
	}
}

// DetectAction classifies the user action from the question text only.
// Entity-gated decisions (commodity present, ship buy vs trade) remain in QueryPlan.
func DetectAction(question string) ActionMatch {
	q := strings.TrimSpace(question)
	if q == "" {
		return ActionMatch{Action: ActionNone, Confidence: "low", RuleID: "empty"}
	}
	best := ActionRule{Priority: 1 << 30}
	found := false
	for _, rule := range actionRules() {
		if !rule.Match(q) {
			continue
		}
		if !found || rule.Priority < best.Priority {
			best = rule
			found = true
		}
	}
	if !found {
		return ActionMatch{Action: ActionFacts, Confidence: "low", RuleID: "facts_fallback"}
	}
	conf := "high"
	if best.Action == ActionPrice || best.Action == ActionFacts {
		conf = "medium"
	}
	return ActionMatch{Action: best.Action, Confidence: conf, RuleID: best.ID}
}
