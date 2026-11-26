package query

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var (
	obtainRE         = regexp.MustCompile(`(?i)(哪里买|在哪买|能买到|买得到|多少钱|价格|报价|哪[买卖出]|买点|卖点|获取|怎么得|如何得|掉落|奖励|蓝图|制造|制作|配方|材料|\bbuy\b|\bsell\b|\bprice\b|\bcrafting\b|\bblueprint\b|买)`)
	craftMaterialsRE = regexp.MustCompile(`(?i)(材料|配方|制造|制作|crafting)`)
	// Multi-slot / mechanism questions belong on Router→Worker, not sc_answer bypass.
	answerRouteComplexRE = regexp.MustCompile(`(?i)(手续费|买进|buy\s*in|冷却|cooldown|弃任|abandoned|站点池|site_?pool|有哪些站点|刷法|任务机制|声望要求|敌船|敌舰|lifetime|时效|完整列表|分区站点|pyro\s*region|Region\s*[ABCD])`)
)

var guideIntents = map[string]struct{}{
	"howto": {}, "client_help": {}, "earn_meta": {}, "pledge_meta": {},
	"event_guide": {}, "trade_route": {}, "ship_shop": {},
}

const (
	briefFactLimit  = 5
	fullFactLimit   = 25
	briefPriceLimit = 3
	fullPriceLimit  = 10
	briefDocLimit   = 2
	fullDocLimit    = 5
	relationLimit   = 5
)

const deepOrganizeHint = "有本地证据时：结合 answer_draft 与 evidence 做深度整理，写成更详细的玩家可读说明（结论、分点/小表、版本来源、操作含义）；数值与店名等只能来自证据，勿编造未收录字段。"

// answerHandlers dispatches the plain evidence/intent branches. Pre-checks and
// the blueprint / shop / guide / calc paths run before this table.
var answerHandlers = map[string]answerHandler{
	"facts":         answerFacts,
	"prices":        answerPrices,
	"mission_list":  answerMissions,
	"mission_money": answerMissions,
	"docs":          answerDocs,
	"live_web":      answerLiveWeb,
	"live_local":    answerLiveLocal,
	"data_sync":     answerDataSync,
}

// AnswerQuestion is the Go sc_answer implementation (parity with Python answer_question).
func AnswerQuestion(db *sql.DB, question, detailLevel, hintEntity string) (map[string]any, error) {
	ctx, err := newAnswerContext(db, question, detailLevel, hintEntity)
	if err != nil {
		return nil, err
	}

	if shouldDeferAnswerToRoute(ctx.Question, ctx.Intent) {
		return answerDeferToRoute(ctx), nil
	}
	if obtainRE.MatchString(ctx.Question) && ctx.PrimaryKind == "blueprint" && ctx.Primary != "" {
		return answerBlueprintObtain(ctx)
	}
	if obtainRE.MatchString(ctx.Question) && (ctx.PrimaryKind == "ship" || ctx.PrimaryKind == "item") && ctx.Primary != "" && ctx.Intent == "facts" {
		return answerShopObtain(ctx)
	}
	if _, ok := guideIntents[ctx.Intent]; ok {
		return answerGuideIntent(ctx)
	}
	if ctx.Intent == "loadout_calc" || ctx.Intent == "mining_calc" {
		return answerCalcIntent(ctx)
	}

	var res answerResult
	handler, ok := answerHandlers[ctx.Intent]
	if ok {
		res, err = handler(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		res = newAnswerResult(ctx.evidence())
		res.Draft = "本地知识库未覆盖该意图；不会编造数据。"
		res.Missing = append(res.Missing, "intent_handler")
	}
	res.Evidence["missing"] = res.Missing
	if res.Draft == "" {
		res.Draft = "本地知识库未覆盖该问题；不会编造数据。"
	}

	facts := anyx.Maps(res.Evidence["facts"])
	prices := anyx.Maps(res.Evidence["prices"])
	docs := anyx.Maps(res.Evidence["docs"])
	conf := recomputeConfidence(ctx.Intent, anyx.String(ctx.Plan["confidence"]), res.HadScored,
		len(ctx.Entities) > 0, facts, prices, docs, res.Missing)
	browser := ctx.browser()
	if needsSoftVerify(ctx.Intent, conf, res.HadScored, facts, prices, docs) {
		soft := "要用浏览器核实吗？"
		has := false
		for _, f := range res.FollowUps {
			if f == soft {
				has = true
				break
			}
		}
		if !has {
			res.FollowUps = append(res.FollowUps, soft)
		}
		if ctx.Intent == "facts" || ctx.Intent == "prices" || ctx.Intent == "docs" {
			browser = map[string]any{"needed": false}
		}
	}
	return answerOut(ctx.Intent, ctx.Path, conf, ctx.Live, res.Draft, res.Evidence, res.Detail, res.FollowUps, browser, ctx.Plan), nil
}

// answerOut projects the shared answer shape onto the MCP response map.
func answerOut(intent, path, conf, live, draft string, evidence map[string]any, detail bool, followUps []string, browser, plan map[string]any) map[string]any {
	return map[string]any{
		"intent": intent, "path": path, "confidence": conf,
		"live_game_version": live, "answer_draft": draft, "evidence": evidence,
		"detail_available": detail, "follow_ups": followUps, "browser": browser, "plan": plan,
	}
}

func shouldDeferAnswerToRoute(question, intent string) bool {
	q := strings.TrimSpace(question)
	if q == "" {
		return false
	}
	if !answerRouteComplexRE.MatchString(q) {
		return false
	}
	// Simple single-commodity price asks stay on sc_answer even if wording overlaps weakly.
	if intent == "prices" && !regexp.MustCompile(`(?i)(手续费|冷却|站点|任务|刷法|声望)`).MatchString(q) {
		return false
	}
	return true
}

func answerDeferToRoute(ctx *answerContext) map[string]any {
	primary := SuggestedPrimary(ClassificationContext{Intent: ctx.Intent, Question: ctx.Question, Entities: ctx.Entities})
	draft := fmt.Sprintf(
		"此题超出 sc_answer 旁路范围（多槽/机制字段），请改用 sc_route → sc_worker_run（建议 primary=%s）。",
		primary,
	)
	evidence := emptyEvidence()
	evidence["missing"] = []string{"use_sc_route"}
	evidence["reasoning_hint"] = "route_required: 用 Router 展开 DAG 后再 Worker 拉 cost/cooldown/site_pool 等证据成文"
	out := answerOut(ctx.Intent, "router_worker", "low", ctx.Live, draft, evidence, false,
		[]string{"改用 sc_route"}, map[string]any{"needed": false}, ctx.Plan)
	out["route_required"] = true
	out["suggested_primary"] = primary
	return out
}

func emptyEvidence() map[string]any {
	return map[string]any{
		"entities": []map[string]any{}, "facts": []map[string]any{}, "relations": []map[string]any{},
		"prices": []map[string]any{}, "docs": []map[string]any{}, "missing": []string{},
	}
}

func displayName(entities []map[string]any) string {
	if len(entities) == 0 {
		return ""
	}
	e := entities[0]
	name := anyx.String(e["canonical_name"])
	if zh := anyx.String(e["zh"]); zh != "" {
		return fmt.Sprintf("%s（%s）", zh, name)
	}
	return name
}

func orLabel(label, fallback string) string {
	if label != "" {
		return label
	}
	return fallback
}
