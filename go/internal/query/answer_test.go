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

func TestAnswerQuestionGuideUsesBridge(t *testing.T) {
	root := planRepoRoot(t)
	SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skip(err)
	}
	defer sqlDB.Close()

	_, err = AnswerQuestion(sqlDB, "萌新怎么赚钱开荒", "brief", "")
	if err != ErrUseBridge {
		t.Fatalf("want ErrUseBridge, got %v", err)
	}
}
