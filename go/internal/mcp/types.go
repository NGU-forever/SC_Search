package mcp

// Typed MCP tool inputs — go-sdk derives JSON Schema from these structs.

type ResolveNameIn struct {
	Text string `json:"text" jsonschema:"Chinese/English/slang name to resolve"`
}

type BestPricesIn struct {
	Commodity string `json:"commodity" jsonschema:"Commodity name (CN/EN/alias)"`
	Side      string `json:"side,omitempty" jsonschema:"buy or sell; default sell"`
	System    string `json:"system,omitempty" jsonschema:"Optional star system filter (Stanton/Pyro/Nyx or 中文)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max rows; default 10"`
}

type TerminalPricesIn struct {
	Terminal string `json:"terminal" jsonschema:"Terminal / shop display name"`
	Side     string `json:"side,omitempty" jsonschema:"Optional buy or sell filter"`
}

type DataFreshnessIn struct {
	Source string `json:"source,omitempty" jsonschema:"Optional ingest source filter (uex/scmdb/wiki/...)"`
}

type EntityFactsIn struct {
	NameOrID string `json:"name_or_id" jsonschema:"Entity name, alias, or numeric id"`
	Detail   string `json:"detail,omitempty" jsonschema:"compact (default) or full"`
}

type LocationsIn struct {
	Query        string `json:"query,omitempty" jsonschema:"Optional location name substring"`
	System       string `json:"system,omitempty" jsonschema:"Optional star system filter"`
	LocationType string `json:"location_type,omitempty" jsonschema:"Optional location type filter"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Max rows; default 20"`
}

type SearchDocsIn struct {
	Query string `json:"query" jsonschema:"Wiki/doc search terms"`
	Limit int    `json:"limit,omitempty" jsonschema:"Max snippets; default 5"`
}

type ListMissionsIn struct {
	Faction     string `json:"faction,omitempty" jsonschema:"Faction / contractor name"`
	RankKeyword string `json:"rank_keyword,omitempty" jsonschema:"Rank or activity keyword (English preferred)"`
	System      string `json:"system,omitempty" jsonschema:"Optional star system filter"`
	Limit       int    `json:"limit,omitempty" jsonschema:"Max rows; default 50"`
}

type QueryPlanIn struct {
	Question   string `json:"question" jsonschema:"User question"`
	HintEntity string `json:"hint_entity,omitempty" jsonschema:"Optional entity hint"`
}

type AnswerIn struct {
	Question    string `json:"question" jsonschema:"User question"`
	DetailLevel string `json:"detail_level,omitempty" jsonschema:"brief (default) or full"`
	HintEntity  string `json:"hint_entity,omitempty" jsonschema:"Optional entity hint"`
}

type EmptyIn struct{}

type RouteIn struct {
	Question    string         `json:"question" jsonschema:"User question"`
	Primary     string         `json:"primary,omitempty" jsonschema:"Router primary from sc_intents"`
	Slots       map[string]any `json:"slots,omitempty" jsonschema:"Slot key-value object"`
	SlotsJSON   string         `json:"slots_json,omitempty" jsonschema:"Optional JSON string form of slots (legacy)"`
	ReplanCount int            `json:"replan_count,omitempty" jsonschema:"Replan attempt count; default 0"`
}

type WorkerRunIn struct {
	Plan     map[string]any `json:"plan,omitempty" jsonschema:"DAG plan object from sc_route data"`
	PlanJSON string         `json:"plan_json,omitempty" jsonschema:"Optional JSON string form of plan (legacy)"`
}

type AskIn struct {
	Question    string         `json:"question" jsonschema:"User question"`
	Primary     string         `json:"primary,omitempty" jsonschema:"Optional primary override"`
	Slots       map[string]any `json:"slots,omitempty" jsonschema:"Optional slot overrides"`
	SlotsJSON   string         `json:"slots_json,omitempty" jsonschema:"Legacy JSON string form of slots"`
	DetailLevel string         `json:"detail_level,omitempty" jsonschema:"brief (default) or full"`
	TraceLevel  string         `json:"trace_level,omitempty" jsonschema:"none, compact (default), or full"`
}
