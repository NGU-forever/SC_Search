package query

import (
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
)

func TestAnswerQuestionFacts(t *testing.T) {
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	ans, err := AnswerQuestion(sqlDB, "卡拉克货舱多大", "brief", "")
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
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	ans, err := AnswerQuestion(sqlDB, "萌新怎么赚钱开荒", "brief", "")
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
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	ans, err := AnswerQuestion(sqlDB, "Bit Zeros ship heist 手续费和冷却，站点有哪些", "brief", "")
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
