package ingest

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/frankxiong/sc-search/internal/query"
)

type seedAlias struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
	Lang   string `json:"lang"`
	Kind   string `json:"kind"`
}

func LoadSeedAliases(path string) ([]seedAlias, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []seedAlias
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func ApplySeedAliases(conn *sql.DB, entries []seedAlias, gameVersion string) (int, error) {
	if gameVersion == "" {
		gameVersion = "aliases"
	}
	count := 0
	for _, entry := range entries {
		alias := strings.TrimSpace(entry.Alias)
		target := strings.TrimSpace(entry.Target)
		lang := strings.TrimSpace(entry.Lang)
		kind := strings.TrimSpace(entry.Kind)
		if lang == "" {
			lang = "zh"
		}
		if kind == "" {
			kind = "ship"
		}
		if alias == "" || target == "" {
			continue
		}
		matches, err := query.ResolveName(conn, target)
		if err != nil {
			return count, err
		}
		var entityID int64
		pool := []map[string]any{}
		for _, m := range matches {
			cn := strings.EqualFold(strAny(m["canonical_name"]), target)
			ma := strings.EqualFold(strAny(m["matched_alias"]), target)
			if cn || ma {
				pool = append(pool, m)
			}
		}
		if len(pool) == 0 {
			pool = matches
		}
		kindHits := []map[string]any{}
		for _, m := range pool {
			if strAny(m["kind"]) == kind {
				kindHits = append(kindHits, m)
			}
		}
		if len(kindHits) > 0 {
			entityID = asInt64Any(kindHits[0]["id"])
		} else {
			entityID, err = UpsertEntity(conn, kind, target, gameVersion, "", "alias-target-"+kind+"-"+target)
			if err != nil {
				return count, err
			}
			if err := UpsertAlias(conn, entityID, target, "en", "seed"); err != nil {
				return count, err
			}
		}
		if err := UpsertAlias(conn, entityID, alias, lang, "seed"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// SyncAliases loads seed_aliases.json and writes alias rows (CitizenWiki probe skipped in Go for now).
func SyncAliases(conn *sql.DB, repoRoot, seedPath string) (int, error) {
	if seedPath == "" {
		seedPath = filepath.Join(repoRoot, "data", "seed_aliases.json")
	}
	runID, err := StartRun(conn, "aliases", "")
	if err != nil {
		return 0, err
	}
	entries, err := LoadSeedAliases(seedPath)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	count, err := ApplySeedAliases(conn, entries, "aliases")
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", "aliases"); err != nil {
		return count, err
	}
	return count, nil
}

func strAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asInt64Any(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}
