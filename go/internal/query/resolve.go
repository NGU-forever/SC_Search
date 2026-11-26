package query

import (
	"sort"
	"strings"

	"github.com/frankxiong/sc-search/internal/anyx"
)

var kindRank = map[string]int{
	"ship":           0,
	"location":       1,
	"commodity":      2,
	"terminal":       3,
	"item":           4,
	"faction":        5,
	"blueprint":      6,
	"blueprint_pool": 7,
	"mission":        8,
}

func ResolveName(db Queryer, text string) ([]map[string]any, error) {
	q := strings.TrimSpace(text)
	if q == "" {
		return []map[string]any{}, nil
	}

	rows, err := db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       a.alias AS matched_alias, a.lang AS matched_lang
		FROM aliases a
		JOIN entities e ON e.id = a.entity_id
		WHERE a.alias = ? COLLATE NOCASE
	`, q)
	if err != nil {
		return nil, err
	}
	items, err := scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return rankRows(items), nil
	}

	rows, err = db.Query(`
		SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       e.canonical_name AS matched_alias, 'en' AS matched_lang
		FROM entities e
		WHERE e.canonical_name = ? COLLATE NOCASE
	`, q)
	if err != nil {
		return nil, err
	}
	items, err = scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return rankRows(items), nil
	}

	like := "%" + q + "%"
	rows, err = db.Query(`
		SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       a.alias AS matched_alias, a.lang AS matched_lang
		FROM aliases a
		JOIN entities e ON e.id = a.entity_id
		WHERE a.alias LIKE ? COLLATE NOCASE
		LIMIT 50
	`, like)
	if err != nil {
		return nil, err
	}
	items, err = scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		ranked := rankRows(items)
		if len(ranked) > 25 {
			ranked = ranked[:25]
		}
		return ranked, nil
	}

	rows, err = db.Query(`
		SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
		       e.canonical_name AS matched_alias, 'en' AS matched_lang
		FROM entities e
		WHERE e.canonical_name LIKE ? COLLATE NOCASE
		LIMIT 50
	`, like)
	if err != nil {
		return nil, err
	}
	items, err = scanMaps(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	ranked := rankRows(items)
	if len(ranked) > 25 {
		ranked = ranked[:25]
	}
	return ranked, nil
}

func rankRows(items []map[string]any) []map[string]any {
	rankOf := func(kind string) int {
		if r, ok := kindRank[kind]; ok {
			return r
		}
		return 99
	}
	sort.SliceStable(items, func(i, j int) bool {
		ki := rankOf(anyx.String(items[i]["kind"]))
		kj := rankOf(anyx.String(items[j]["kind"]))
		if ki != kj {
			return ki < kj
		}
		return anyx.String(items[i]["canonical_name"]) < anyx.String(items[j]["canonical_name"])
	})
	return items
}
