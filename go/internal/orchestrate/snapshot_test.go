package orchestrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/frankxiong/sc-search/internal/query"
	"github.com/frankxiong/sc-search/internal/testfixture"
)

// TestCharacterizationSnapshot locks the observable output of the query and
// orchestration entry points against a deterministic fixture database. The
// snapshot is generated from the same eval questions used by the golden eval,
// so refactors that change classification, SQL, wording, or envelope shape
// fail here before they reach a live database.
//
// Run with UPDATE_SNAPSHOTS=1 to regenerate go/internal/orchestrate/testdata/snapshots.
func TestCharacterizationSnapshot(t *testing.T) {
	root := repoRoot(t)
	query.SetGuidesRoot(testfixture.Root(t))
	questions := loadGoldCases(t, filepath.Join(root, "eval", "fixture_questions.jsonl"))
	if len(questions) == 0 {
		t.Fatal("no fixture questions loaded")
	}

	update := os.Getenv("UPDATE_SNAPSHOTS") == "1"
	dir := filepath.Join(root, "go", "internal", "orchestrate", "testdata", "snapshots")
	if update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	engines := []struct {
		name string
		opts testfixture.Options
	}{
		{name: "fresh", opts: testfixture.Options{UEXGameVersion: testfixture.FreshUEXVersion}},
		{name: "stale", opts: testfixture.Options{UEXGameVersion: testfixture.StaleUEXVersion}},
	}
	for _, eng := range engines {
		eng := eng
		t.Run(eng.name, func(t *testing.T) {
			conn := testfixture.NewWithOptions(t, eng.opts)
			engine := NewEngine(conn)
			cases := make([]snapshotCase, 0, len(questions))
			for _, q := range questions {
				ask := engine.Ask(context.Background(), AskInput{
					Question:   q.Question,
					TraceLevel: "full",
				})
				plan, err := query.QueryPlan(conn, q.Question, "")
				if err != nil {
					t.Fatalf("%s: QueryPlan: %v", q.Question, err)
				}
				answer, err := query.AnswerQuestion(conn, q.Question, "brief", "")
				if err != nil {
					t.Fatalf("%s: AnswerQuestion: %v", q.Question, err)
				}
				cases = append(cases, snapshotCase{
					Question: q.Question,
					Ask:      normalizeSnapshot(ask),
					Plan:     normalizeSnapshot(plan),
					Answer:   normalizeSnapshot(answer),
				})
			}
			raw, err := json.MarshalIndent(cases, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			raw = append(raw, '\n')
			path := filepath.Join(dir, eng.name+".json")
			if update {
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read snapshot %s: %v (run with UPDATE_SNAPSHOTS=1 to create)", path, err)
			}
			if !bytes.Equal(raw, want) {
				t.Fatalf("snapshot mismatch for %s\n%s", path, firstDiff(want, raw))
			}
		})
	}
}

type snapshotCase struct {
	Question string `json:"question"`
	Ask      any    `json:"ask"`
	Plan     any    `json:"plan"`
	Answer   any    `json:"answer"`
}

// normalizeSnapshot converts a result into a deterministic JSON value:
// encoding/json sorts map keys, and volatile fields (timings, timestamps,
// ingest run IDs, database entity IDs) are removed because fixture entity IDs
// depend on Go map iteration order during ingest.
func normalizeSnapshot(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("snapshot marshal: %v", err))
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(fmt.Sprintf("snapshot unmarshal: %v", err))
	}
	return stripVolatile(out)
}

func stripVolatile(v any) any {
	switch val := v.(type) {
	case string:
		return snapshotTimestampRE.ReplaceAllString(val, "<timestamp>")
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			if volatileSnapshotKey(k) {
				continue
			}
			out[k] = stripVolatile(item)
		}
		for _, key := range []string{"sources", "stale_sources", "game_versions"} {
			sortStringSlice(out[key])
		}
		return out
	case []any:
		sortEvidence := workerEvidenceSlice(val)
		out := make([]any, 0, len(val))
		for _, item := range val {
			out = append(out, stripVolatile(item))
		}
		if sortEvidence {
			sort.SliceStable(out, func(i, j int) bool {
				return canonicalJSON(out[i]) < canonicalJSON(out[j])
			})
		}
		return out
	default:
		return v
	}
}

var snapshotTimestampRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?`)

func sortStringSlice(v any) {
	items, ok := v.([]any)
	if !ok {
		return
	}
	strs := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return
		}
		strs = append(strs, s)
	}
	sort.Strings(strs)
	for i, s := range strs {
		items[i] = s
	}
}

// workerEvidenceSlice matches the worker's evidence rows (id/tool/ok/meta) whose
// order follows parallel wave completion. Query answer evidence keeps its order.
func workerEvidenceSlice(items []any) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		for _, key := range []string{"id", "tool", "ok", "meta"} {
			if _, ok := m[key]; !ok {
				return false
			}
		}
	}
	return true
}

func canonicalJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func volatileSnapshotKey(key string) bool {
	k := strings.ToLower(key)
	switch k {
	case "duration_ms", "synced_at", "started_at", "finished_at", "updated_at",
		"created_at", "date_modified", "date_added", "id", "ingest_run_id",
		"entity_id", "from_id", "to_id":
		return true
	}
	return strings.HasSuffix(k, "_id")
}

func firstDiff(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("first diff at line %d:\n  want: %s\n  got:  %s", i+1, wantLines[i], gotLines[i])
		}
	}
	return fmt.Sprintf("line count differs: want %d, got %d", len(wantLines), len(gotLines))
}
