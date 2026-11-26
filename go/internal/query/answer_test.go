package query_test

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

func TestAnswerQuestionFacts(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	sqlDB := testfixture.New(t)

	ans, err := query.AnswerQuestion(sqlDB, "卡拉克货舱多大", "brief", "")
	if err != nil {
		t.Fatal(err)
	}
	if ans["intent"] != "facts" {
		t.Fatalf("intent=%v", ans["intent"])
	}
	draft, _ := ans["answer_draft"].(string)
	if draft == "" {
		t.Fatal("empty draft")
	}
}

func TestAnswerQuestionEarnMetaNative(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	sqlDB := testfixture.New(t)

	ans, err := query.AnswerQuestion(sqlDB, "萌新怎么赚钱开荒", "brief", "")
	if err != nil {
		t.Fatal(err)
	}
	if ans["intent"] != "earn_meta" {
		t.Fatalf("intent=%v", ans["intent"])
	}
	draft, _ := ans["answer_draft"].(string)
	if draft == "" {
		t.Fatal("empty draft")
	}
}

func TestAnswerQuestionDefersComplexMission(t *testing.T) {
	query.SetGuidesRoot(testfixture.Root(t))
	sqlDB := testfixture.New(t)

	ans, err := query.AnswerQuestion(sqlDB, "Bit Zeros ship heist 手续费和冷却，站点有哪些", "brief", "")
	if err != nil {
		t.Fatal(err)
	}
	if ans["route_required"] != true {
		t.Fatalf("expected route_required, got %#v", ans)
	}
	if ans["suggested_primary"] == nil || ans["suggested_primary"] == "" {
		t.Fatalf("missing suggested_primary: %#v", ans)
	}
	if ans["path"] != "router_worker" {
		t.Fatalf("path=%v", ans["path"])
	}
}
