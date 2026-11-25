package orchestrate

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type goldCase struct {
	Question        string   `json:"question"`
	ExpectedPrimary string   `json:"expected_primary"`
	MustContain     []string `json:"must_contain"`
	MustNotContain  []string `json:"must_not_contain"`
	MustNotBrowse   bool     `json:"must_not_browse"`
	MustWaitConfirm bool     `json:"must_wait_confirm"`
}

func TestAskGoldQuestionsStrict(t *testing.T) {
	eng := testEngine(t)
	path := filepath.Join(repoRoot(t), "eval", "questions.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	i := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		i++
		var c goldCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("#%d parse: %v", i, err)
		}
		res := eng.Ask(context.Background(), AskInput{Question: c.Question, TraceLevel: "compact"})
		text := res.AnswerDraft + " " + res.AnswerBlocks.Headline
		for _, fact := range res.AnswerBlocks.Facts {
			text += " " + fact.Label + " " + fact.Value
		}
		var reasons []string
		if c.ExpectedPrimary != "" && res.Primary != c.ExpectedPrimary {
			reasons = append(reasons, "primary got="+res.Primary+" want="+c.ExpectedPrimary)
		}
		for _, m := range c.MustContain {
			if !strings.Contains(text, m) {
				reasons = append(reasons, "missing "+m)
			}
		}
		for _, m := range c.MustNotContain {
			if strings.Contains(text, m) {
				reasons = append(reasons, "forbidden "+m)
			}
		}
		if c.MustNotBrowse && res.Browser.Needed {
			reasons = append(reasons, "browser.needed=true")
		}
		if c.MustWaitConfirm && (!res.Browser.Needed || !res.Browser.WaitConfirm) {
			reasons = append(reasons, "want wait_confirm")
		}
		if res.Status == StatusFailed && res.Error == nil {
			reasons = append(reasons, "failed without error")
		}
		if res.Status == StatusPartial && len(res.AnswerBlocks.Missing) == 0 && len(res.AnswerBlocks.Warnings) == 0 {
			reasons = append(reasons, "partial without missing/warning")
		}
		if len(reasons) > 0 {
			t.Errorf("#%d q=%q primary=%s mode=%s status=%s\n  %s\n  draft=%q",
				i, c.Question, res.Primary, res.Trace.Mode, res.Status,
				strings.Join(reasons, "; "), truncate(res.AnswerDraft, 200))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if i == 0 {
		t.Fatal("no gold questions loaded")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
