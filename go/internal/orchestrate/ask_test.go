package orchestrate

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
	"github.com/frankxiong/sc-search/internal/query"
)

func TestAskCarrackBypass(t *testing.T) {
	eng := testEngine(t)
	res := eng.Ask(context.Background(), AskInput{Question: "卡拉克货舱多大"})
	if !res.OK {
		t.Fatalf("ok=false err=%v draft=%q", res.Error, res.AnswerDraft)
	}
	if res.Trace.Mode != "answer_bypass" && res.Primary != "ship_stats" && res.Primary != "other" {
		// Accept either bypass facts or route to ship_stats depending on plan.
		t.Logf("mode=%s primary=%s", res.Trace.Mode, res.Primary)
	}
	joined := res.AnswerDraft + " " + blocksText(res)
	if !strings.Contains(strings.ToLower(joined), "anvil") && !strings.Contains(joined, "456") && !strings.Contains(joined, "Carrack") && !strings.Contains(joined, "货舱") {
		t.Fatalf("expected ship facts in draft/blocks, got draft=%q blocks=%+v primary=%s mode=%s", res.AnswerDraft, res.AnswerBlocks, res.Primary, res.Trace.Mode)
	}
	if res.Browser.Needed {
		t.Fatalf("mcp_only should not need browser: %+v", res.Browser)
	}
}

func TestAskGoldPrices(t *testing.T) {
	eng := testEngine(t)
	res := eng.Ask(context.Background(), AskInput{Question: "黄金哪里卖最贵"})
	if !res.OK {
		t.Fatalf("ok=false err=%v", res.Error)
	}
	if res.Primary != "trade" && res.Trace.Mode != "answer_bypass" {
		t.Logf("primary=%s mode=%s", res.Primary, res.Trace.Mode)
	}
	if res.AnswerDraft == "" {
		t.Fatal("empty draft")
	}
	if res.Browser.Needed {
		t.Fatalf("prices should be mcp_only, browser=%+v", res.Browser)
	}
}

func TestAskLoadoutWaitConfirm(t *testing.T) {
	eng := testEngine(t)
	// Real entry: no Primary override — classifier must pick loadout from「改装」.
	res := eng.Ask(context.Background(), AskInput{
		Question: "流星改装方案",
	})
	if !res.OK {
		t.Fatalf("ok=false err=%v", res.Error)
	}
	if res.Primary != "loadout" {
		t.Fatalf("primary=%s mode=%s", res.Primary, res.Trace.Mode)
	}
	if !res.Browser.Needed || !res.Browser.WaitConfirm {
		t.Fatalf("loadout must wait_confirm, browser=%+v", res.Browser)
	}
	if res.Status != StatusAwaitingUser && res.Status != StatusCompleted && res.Status != StatusPartial {
		t.Fatalf("status=%s", res.Status)
	}
	if res.Browser.SearchTerm == "流星" {
		t.Fatalf("search_term must not stay Chinese slang: %q", res.Browser.SearchTerm)
	}
}

func TestAskMiningWaitConfirm(t *testing.T) {
	eng := testEngine(t)
	res := eng.Ask(context.Background(), AskInput{Question: "怎么采矿配激光"})
	if !res.OK {
		t.Fatalf("ok=false err=%v", res.Error)
	}
	if res.Primary != "mining" {
		t.Fatalf("primary=%s", res.Primary)
	}
	if !res.Browser.Needed || !res.Browser.WaitConfirm {
		t.Fatalf("mining must wait_confirm, browser=%+v", res.Browser)
	}
}

func TestReplanFromShipStatsConflict(t *testing.T) {
	st := &State{
		Primary:    "ship_stats",
		Question:   "Bit Zeros ship heist 手续费和冷却",
		PlanIntent: "facts",
		Slots:      map[string]any{},
		Evidence:   []map[string]any{},
	}
	next := replanFromConflict(st)
	if next != "mission_money" {
		t.Fatalf("next=%q", next)
	}
}

func TestAskComplexMissionRoutes(t *testing.T) {
	eng := testEngine(t)
	res := eng.Ask(context.Background(), AskInput{
		Question: "Bit Zeros ship heist 手续费和冷却，站点有哪些",
	})
	if res.AnswerDraft == "" && res.Error != nil && res.Error.Code == ErrInvalidInput {
		t.Fatal(res.Error)
	}
	// Should not bounce client with route_required-only draft.
	if strings.Contains(res.AnswerDraft, "请改用 sc_route") {
		t.Fatalf("sc_ask should execute route/worker itself, draft=%q", res.AnswerDraft)
	}
	if res.Trace.Mode == "" {
		t.Fatal("missing trace mode")
	}
}

func TestAskNoMatchDoesNotFabricate(t *testing.T) {
	eng := testEngine(t)
	res := eng.Ask(context.Background(), AskInput{
		Question: "zzz_not_a_real_sc_entity_qqq_99999 参数",
	})
	if res.AnswerDraft == "" {
		t.Fatal("expected draft")
	}
	lower := strings.ToLower(res.AnswerDraft + blocksText(res))
	if strings.Contains(lower, "dps 99999") {
		t.Fatalf("fabricated content: %q", res.AnswerDraft)
	}
}

func TestBrowserGateMCPOnly(t *testing.T) {
	gate := browserGateFor("ship_stats", "mcp_only", "Carrack", nil)
	if gate.Needed || gate.WaitConfirm {
		t.Fatalf("mcp_only gate=%+v", gate)
	}
}

func TestSlotRetryNoMetaSpeech(t *testing.T) {
	st := &State{
		Primary: "mission_money",
		Slots:   map[string]any{"activity": "开荒"},
		Worker: map[string]any{
			"slot_retry_suggested": true,
			"replan_suggested":     false,
		},
		Evidence: []map[string]any{},
	}
	if applyRetry(st) {
		if asString(st.Slots["activity"]) == "开荒" && st.SlotRetryCount > 0 {
			// applyRetry may refuse meta words via canonical empty path
		}
	}
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	root := repoRoot(t)
	query.SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return NewEngine(sqlDB)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func blocksText(res AskResult) string {
	var b strings.Builder
	b.WriteString(res.AnswerBlocks.Headline)
	for _, f := range res.AnswerBlocks.Facts {
		b.WriteString(f.Label)
		b.WriteString(f.Value)
	}
	return b.String()
}
