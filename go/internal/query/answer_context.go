package query

import (
	"database/sql"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

// answerContext carries the shared, per-question inputs of one sc_answer call.
// Handlers only read it; no package-level mutable state is involved.
type answerContext struct {
	DB          *sql.DB
	Question    string
	Level       string
	Plan        map[string]any
	Intent      string
	Path        string
	Entities    []map[string]any
	Primary     string
	PrimaryKind string
	Label       string
	Live        string
	FactLimit   int
	PriceLimit  int
	DocLimit    int
}

// answerResult mirrors the per-branch pieces that AnswerQuestion used to keep
// in local variables before the dispatch split.
type answerResult struct {
	Draft     string
	Evidence  map[string]any
	Missing   []string
	FollowUps []string
	Detail    bool
	Browser   map[string]any
	HadScored bool
}

type answerHandler func(*answerContext) (answerResult, error)

func newAnswerContext(db *sql.DB, question, detailLevel, hintEntity string) (*answerContext, error) {
	level := strings.ToLower(strings.TrimSpace(detailLevel))
	if level != "brief" && level != "full" {
		level = "brief"
	}
	plan, err := QueryPlan(db, question, hintEntity)
	if err != nil {
		return nil, err
	}
	ctx := &answerContext{
		DB:       db,
		Question: question,
		Level:    level,
		Plan:     plan,
	}
	ctx.Intent, _ = plan["intent"].(string)
	ctx.Path, _ = plan["path"].(string)
	ctx.Entities, _ = plan["entities"].([]map[string]any)
	if raw, ok := plan["entities"].([]any); ok {
		ctx.Entities = anyx.Maps(raw)
	}
	if len(ctx.Entities) > 0 {
		ctx.Primary = anyx.String(ctx.Entities[0]["canonical_name"])
		ctx.PrimaryKind = anyx.String(ctx.Entities[0]["kind"])
	}
	ctx.FactLimit = briefFactLimit
	ctx.PriceLimit = briefPriceLimit
	ctx.DocLimit = briefDocLimit
	if level == "full" {
		ctx.FactLimit = fullFactLimit
		ctx.PriceLimit = fullPriceLimit
		ctx.DocLimit = fullDocLimit
	}
	ctx.Live, _ = LiveGameVersion(db)
	ctx.Label = displayName(ctx.Entities)
	return ctx, nil
}

// evidence returns the standard evidence envelope seeded with this question's
// entities.
func (c *answerContext) evidence() map[string]any {
	ev := emptyEvidence()
	ev["entities"] = c.Entities
	return ev
}

// browser returns the plan's browser block, defaulting to "not needed".
func (c *answerContext) browser() map[string]any {
	browser, _ := c.Plan["browser"].(map[string]any)
	if browser == nil {
		browser = map[string]any{"needed": false}
	}
	return browser
}

// newAnswerResult starts a result with non-nil empty slices so the JSON shape
// keeps [] rather than null.
func newAnswerResult(evidence map[string]any) answerResult {
	return answerResult{
		Evidence:  evidence,
		Missing:   []string{},
		FollowUps: []string{},
	}
}
