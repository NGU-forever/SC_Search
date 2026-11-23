package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

var (
	guidesOnce sync.Once
	guidesData map[string][]map[string]string
	guidesErr  error
	guidesRoot string
)

// SetGuidesRoot sets the repo root used to load data/community_guides.json.
func SetGuidesRoot(root string) {
	guidesRoot = root
	guidesOnce = sync.Once{}
	guidesData = nil
	guidesErr = nil
}

func loadGuides() (map[string][]map[string]string, error) {
	guidesOnce.Do(func() {
		root := guidesRoot
		if root == "" {
			wd, err := os.Getwd()
			if err != nil {
				guidesErr = err
				return
			}
			root = wd
		}
		path := filepath.Join(root, "data", "community_guides.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			// Try parent if running from go/
			path2 := filepath.Join(filepath.Dir(root), "data", "community_guides.json")
			raw, err = os.ReadFile(path2)
			if err != nil {
				guidesErr = err
				return
			}
		}
		var parsed map[string][]map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			guidesErr = err
			return
		}
		out := map[string][]map[string]string{}
		for key, rows := range parsed {
			cleaned := []map[string]string{}
			for _, row := range rows {
				title, _ := row["title"].(string)
				url, _ := row["url"].(string)
				kind, _ := row["kind"].(string)
				if kind == "" {
					kind = "link"
				}
				if title != "" && url != "" {
					cleaned = append(cleaned, map[string]string{
						"title": title, "url": url, "kind": kind,
					})
				}
			}
			if len(cleaned) > 0 {
				out[key] = cleaned
			}
		}
		guidesData = out
	})
	return guidesData, guidesErr
}

func GuidesFor(category string, limit int) []map[string]string {
	if limit < 0 {
		limit = 0
	}
	data, err := loadGuides()
	if err != nil || data == nil {
		return nil
	}
	rows := data[category]
	if len(rows) > limit {
		return append([]map[string]string{}, rows[:limit]...)
	}
	return append([]map[string]string{}, rows...)
}

func BrowserGuide(category string, limit int) map[string]any {
	if limit <= 0 {
		limit = 4
	}
	resources := GuidesFor(category, limit)
	start := ""
	if len(resources) > 0 {
		start = resources[0]["url"]
	}
	return map[string]any{
		"needed":      true,
		"site":        "guide",
		"start_url":   start,
		"search_term": "",
		"resources":   resources,
		"max_actions": 6,
		"extract":     []any{"page title", "key steps if visible"},
		"do_not":      []any{"open /auth", "/settings", "login", "wander other sites first"},
		"wait_confirm": true,
	}
}
