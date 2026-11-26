package orchestrate

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frankxiong/sc-search/internal/db"
	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

type goldCase struct {
	Question           string   `json:"question"`
	ExpectedPrimary    string   `json:"expected_primary"`
	ExpectedStatus     string   `json:"expected_status"`
	MustContain        []string `json:"must_contain"`
	MustContainAny     []string `json:"must_contain_any"`
	MustNotContain     []string `json:"must_not_contain"`
	MustHaveFactLabels []string `json:"must_have_fact_labels"`
	MustNotBrowse      bool     `json:"must_not_browse"`
	MustWaitConfirm    bool     `json:"must_wait_confirm"`
	Tags               []string `json:"tags"`
}

// TestAskGoldQuestionsStrict is the live-database eval (local only). It skips
// cleanly when data/sc_knowledge.sqlite is absent; CI uses the fixture eval.
func TestAskGoldQuestionsStrict(t *testing.T) {
	root := repoRoot(t)
	query.SetGuidesRoot(root)
	sqlDB, err := db.Open(root)
	if err != nil {
		t.Skipf("live DB not present (%v); fixture eval covers CI", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	eng := NewEngine(sqlDB)

	runGoldQuestions(t, filepath.Join(root, "eval", "questions.jsonl"), func(goldCase) *Engine { return eng })
}

// TestAskFixtureGoldQuestions is the deterministic CI eval: no live DB required.
func TestAskFixtureGoldQuestions(t *testing.T) {
	root := testfixture.Root(t)
	query.SetGuidesRoot(root)
	engines := map[bool]*Engine{
		false: NewEngine(testfixture.NewWithOptions(t, testfixture.Options{UEXGameVersion: testfixture.FreshUEXVersion})),
		true:  NewEngine(testfixture.NewWithOptions(t, testfixture.Options{UEXGameVersion: testfixture.StaleUEXVersion})),
	}
	path := filepath.Join(repoRoot(t), "eval", "fixture_questions.jsonl")

	cases := loadGoldCases(t, path)
	if len(cases) < 60 {
		t.Errorf("fixture eval has %d questions, want >=60", len(cases))
	}
	counts := map[string]int{}
	seen := map[string]int{}
	anyOnly := 0
	known := map[string]bool{
		"ship": true, "price": true, "mission_fee": true, "mission_reward": true,
		"blueprint": true, "no_match": true, "gate": true, "classification": true, "freshness": true,
	}
	for i, c := range cases {
		category := primaryTag(c.Tags)
		if category == "" {
			t.Errorf("question %q has no category tag: %v", c.Question, c.Tags)
			continue
		}
		if !known[category] {
			t.Errorf("question %q has unknown category tag %q", c.Question, category)
			continue
		}
		if c.ExpectedStatus == "" {
			t.Errorf("question %q has no expected_status", c.Question)
		}
		counts[category]++

		engine := "fresh"
		if hasTag(c.Tags, "stale") {
			engine = "stale"
		}
		key := engine + "\x00" + c.Question
		if first, ok := seen[key]; ok {
			t.Errorf("duplicate question %q for %s engine (#%d and #%d)", c.Question, engine, first, i+1)
		} else {
			seen[key] = i + 1
		}
		if len(c.MustContainAny) > 0 && !overlapsAny(c.MustContain, c.MustContainAny) {
			anyOnly++
		}
	}
	for _, category := range []string{"ship", "price", "mission_fee", "mission_reward", "blueprint", "no_match", "gate", "classification", "freshness"} {
		if counts[category] < 3 {
			t.Errorf("category %q has %d questions, want >=3", category, counts[category])
		}
	}
	if anyOnly < 2 {
		t.Errorf("only %d question(s) test must_contain_any without duplicating must_contain; want >=2", anyOnly)
	}

	runGoldQuestions(t, path, func(c goldCase) *Engine {
		return engines[hasTag(c.Tags, "stale")]
	})
}

func loadGoldCases(t *testing.T, path string) []goldCase {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var cases []goldCase
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c goldCase
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("%s: parse: %v", path, err)
		}
		cases = append(cases, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

func runGoldQuestions(t *testing.T, path string, engineFor func(goldCase) *Engine) {
	t.Helper()
	cases := loadGoldCases(t, path)
	if len(cases) == 0 {
		t.Fatal("no gold questions loaded from " + path)
	}
	for i, c := range cases {
		i, c := i, c
		t.Run(fmt.Sprintf("#%02d", i+1), func(t *testing.T) {
			eng := engineFor(c)
			res := eng.Ask(context.Background(), AskInput{Question: c.Question, TraceLevel: "compact"})
			text := answerText(res)
			var reasons []string
			if c.ExpectedPrimary != "" && res.Primary != c.ExpectedPrimary {
				reasons = append(reasons, "primary got="+res.Primary+" want="+c.ExpectedPrimary)
			}
			if c.ExpectedStatus != "" && res.Status != c.ExpectedStatus {
				reasons = append(reasons, "status got="+res.Status+" want="+c.ExpectedStatus)
			}
			for _, m := range c.MustContain {
				if !strings.Contains(text, m) {
					reasons = append(reasons, "missing "+m)
				}
			}
			if len(c.MustContainAny) > 0 {
				matched := false
				for _, m := range c.MustContainAny {
					if strings.Contains(text, m) {
						matched = true
						break
					}
				}
				if !matched {
					reasons = append(reasons, "missing any of "+strings.Join(c.MustContainAny, " / "))
				}
			}
			for _, m := range c.MustNotContain {
				if strings.Contains(text, m) {
					reasons = append(reasons, "forbidden "+m)
				}
			}
			if len(c.MustHaveFactLabels) > 0 {
				labels := map[string]bool{}
				for _, f := range res.AnswerBlocks.Facts {
					labels[f.Label] = true
				}
				for _, want := range c.MustHaveFactLabels {
					if !labels[want] {
						reasons = append(reasons, "missing fact label "+want)
					}
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
				t.Errorf("q=%q primary=%s mode=%s status=%s tags=%v\n  %s\n  draft=%q",
					c.Question, res.Primary, res.Trace.Mode, res.Status, c.Tags,
					strings.Join(reasons, "; "), truncate(res.AnswerDraft, 300))
			}
		})
	}
}

func answerText(res AskResult) string {
	var b strings.Builder
	b.WriteString(res.AnswerDraft)
	b.WriteString(" ")
	b.WriteString(res.AnswerBlocks.Headline)
	for _, f := range res.AnswerBlocks.Facts {
		b.WriteString(" ")
		b.WriteString(f.Label)
		b.WriteString(" ")
		b.WriteString(f.Value)
	}
	for _, w := range res.AnswerBlocks.Warnings {
		b.WriteString(" ")
		b.WriteString(w)
	}
	for _, m := range res.AnswerBlocks.Missing {
		b.WriteString(" ")
		b.WriteString(m)
	}
	return b.String()
}

func primaryTag(tags []string) string {
	for _, tag := range tags {
		if tag != "stale" {
			return tag
		}
	}
	return ""
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

// overlapsAny reports whether any must_contain_any option is also required
// verbatim by must_contain, which would make the "any" assertion redundant.
func overlapsAny(must, any []string) bool {
	for _, a := range any {
		for _, m := range must {
			if a == m {
				return true
			}
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
