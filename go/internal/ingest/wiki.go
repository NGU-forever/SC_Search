package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/frankxiong/sc-search/internal/anyx"
)

const wikiAPI = "https://starcitizen.tools/api.php"
const wikiUserAgent = "SC_Search-ingest/0.1"
const wikiGameVersion = "wiki-live"
const wikiSource = "starcitizen_tools"

var wikiManufacturers = map[string]struct{}{
	"Aegis": {}, "Anvil": {}, "Origin": {}, "Drake": {}, "MISC": {}, "RSI": {},
	"Crusader": {}, "Argo": {}, "Banu": {}, "Esperia": {}, "Mirai": {}, "Kruger": {},
	"Greycat": {}, "Tumbril": {}, "Gatac": {}, "Musashi": {}, "Consolidated": {},
}

// WikiTitleCandidates builds likely starcitizen.tools titles from a ship name.
func WikiTitleCandidates(canonicalName string) []string {
	name := strings.TrimSpace(canonicalName)
	out := []string{name}
	parts := strings.Fields(name)
	if len(parts) >= 2 {
		if _, ok := wikiManufacturers[parts[0]]; ok {
			out = append(out, strings.Join(parts[1:], " "))
		}
	}
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(out))
	for _, t := range out {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		uniq = append(uniq, t)
	}
	return uniq
}

type WikiDoc struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`
	EntityID *int64 `json:"entity_id,omitempty"`
}

// SyncWikiDocuments writes pre-fetched wiki docs (offline / tests).
func SyncWikiDocuments(conn *sql.DB, documents []WikiDoc) (int, error) {
	runID, err := StartRun(conn, wikiSource, "")
	if err != nil {
		return 0, err
	}
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		if err := ClearSourceDocuments(tx, wikiSource, wikiGameVersion); err != nil {
			return 0, err
		}
		n := 0
		for _, doc := range documents {
			if _, err := InsertDocument(tx, doc.Title, doc.Body, wikiSource, wikiGameVersion, runID, doc.EntityID, doc.URL); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", wikiGameVersion); err != nil {
		return count, err
	}
	return count, nil
}

// SyncWikiLive fetches extracts for ship entities (or provided names) from starcitizen.tools.
func SyncWikiLive(conn *sql.DB, shipNames []string, client *http.Client, delay time.Duration) (int, error) {
	runID, err := StartRun(conn, wikiSource, "")
	if err != nil {
		return 0, err
	}
	type row struct {
		id   *int64
		name string
	}
	var rows []row
	if shipNames == nil {
		rs, err := conn.Query(`
			SELECT id, canonical_name FROM entities
			WHERE kind = 'ship'
			ORDER BY canonical_name
		`)
		if err != nil {
			_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
			return 0, err
		}
		defer rs.Close()
		for rs.Next() {
			var id int64
			var name string
			if err := rs.Scan(&id, &name); err != nil {
				_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
				return 0, err
			}
			idCopy := id
			rows = append(rows, row{id: &idCopy, name: name})
		}
		if err := rs.Err(); err != nil {
			_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
			return 0, err
		}
	} else {
		for _, n := range shipNames {
			rows = append(rows, row{name: n})
		}
	}

	nameToEntity := map[string]*int64{}
	var titles []string
	seen := map[string]struct{}{}
	for _, r := range rows {
		for _, cand := range WikiTitleCandidates(r.name) {
			if _, ok := seen[cand]; ok {
				continue
			}
			seen[cand] = struct{}{}
			titles = append(titles, cand)
			nameToEntity[cand] = r.id
		}
	}

	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	docs, err := fetchWikiExtracts(client, titles, delay)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		if err := ClearSourceDocuments(tx, wikiSource, wikiGameVersion); err != nil {
			return 0, err
		}
		n := 0
		for _, doc := range docs {
			eid := nameToEntity[doc.Title]
			if _, err := InsertDocument(tx, doc.Title, doc.Body, wikiSource, wikiGameVersion, runID, eid, doc.URL); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", wikiGameVersion); err != nil {
		return count, err
	}
	return count, nil
}

func fetchWikiExtracts(client *http.Client, titles []string, delay time.Duration) ([]WikiDoc, error) {
	out := []WikiDoc{}
	for i := 0; i < len(titles); i += 20 {
		end := i + 20
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]
		q := url.Values{}
		q.Set("action", "query")
		q.Set("format", "json")
		q.Set("prop", "extracts|info")
		q.Set("exintro", "1")
		q.Set("explaintext", "1")
		q.Set("inprop", "url")
		q.Set("redirects", "1")
		q.Set("titles", strings.Join(batch, "|"))
		req, err := http.NewRequest(http.MethodGet, wikiAPI+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", wikiUserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("wiki HTTP %d", resp.StatusCode)
		}
		var payload struct {
			Query struct {
				Pages map[string]json.RawMessage `json:"pages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		for _, raw := range payload.Query.Pages {
			var page map[string]any
			if err := json.Unmarshal(raw, &page); err != nil {
				continue
			}
			if _, missing := page["missing"]; missing {
				continue
			}
			extract := strings.TrimSpace(anyx.String(page["extract"]))
			if extract == "" {
				continue
			}
			title := anyx.String(page["title"])
			fullURL := anyx.String(page["fullurl"])
			if fullURL == "" {
				fullURL = "https://starcitizen.tools/" + strings.ReplaceAll(title, " ", "_")
			}
			out = append(out, WikiDoc{Title: title, Body: extract, URL: fullURL})
		}
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	return out, nil
}
