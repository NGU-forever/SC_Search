package query

import (
	"database/sql"
	"fmt"
	"strings"
)

func BestPrices(db *sql.DB, commodity, side string, system string, limit int) ([]map[string]any, error) {
	if side != "buy" && side != "sell" {
		return nil, fmt.Errorf("side must be 'buy' or 'sell'")
	}
	if limit <= 0 {
		limit = 10
	}
	matches, err := ResolveName(db, commodity)
	if err != nil {
		return nil, err
	}
	candidates := commodityCandidates(matches)
	if len(candidates) == 0 {
		return []map[string]any{}, nil
	}

	order := "ASC"
	if side == "sell" {
		order = "DESC"
	}
	sys := strings.TrimSpace(system)
	// When filtering by system, pull a wider pool then match POI/parenthetical names in Go
	// (UEX terminals often omit "Stanton"/"Pyro" in the display name).
	fetchLimit := limit
	if sys != "" {
		fetchLimit = limit * 40
		if fetchLimit < 80 {
			fetchLimit = 80
		}
		if fetchLimit > 500 {
			fetchLimit = 500
		}
	}
	baseSQL := `
		SELECT
		  c.canonical_name AS commodity,
		  t.canonical_name AS terminal,
		  p.side,
		  p.price,
		  p.scu_available,
		  p.reported_at,
		  p.source,
		  p.game_version
		FROM prices p
		JOIN entities c ON c.id = p.commodity_entity_id
		JOIN entities t ON t.id = p.terminal_entity_id
		WHERE p.commodity_entity_id = ?
		  AND p.side = ?
	`
	for _, entity := range candidates {
		sqlText := baseSQL + fmt.Sprintf(" ORDER BY p.price %s LIMIT ?", order)
		args := []any{entity["id"], side, fetchLimit}
		rows, err := db.Query(sqlText, args...)
		if err != nil {
			return nil, err
		}
		items, err := scanMaps(rows)
		rows.Close()
		if err != nil {
			return nil, err
		}
		if sys != "" {
			filtered := make([]map[string]any, 0, limit)
			for _, row := range items {
				if TerminalMatchesSystem(str(row["terminal"]), sys) {
					filtered = append(filtered, row)
					if len(filtered) >= limit {
						break
					}
				}
			}
			items = filtered
		}
		if len(items) > 0 {
			if len(items) > limit {
				items = items[:limit]
			}
			return items, nil
		}
	}
	return []map[string]any{}, nil
}

func TerminalPrices(db *sql.DB, terminal string, side string) ([]map[string]any, error) {
	matches, err := ResolveName(db, terminal)
	if err != nil {
		return nil, err
	}
	entity := pickEntity(matches, "terminal")
	if entity == nil {
		return []map[string]any{}, nil
	}
	sqlText := `
		SELECT
		  c.canonical_name AS commodity,
		  t.canonical_name AS terminal,
		  p.side,
		  p.price,
		  p.scu_available,
		  p.reported_at,
		  p.source,
		  p.game_version
		FROM prices p
		JOIN entities c ON c.id = p.commodity_entity_id
		JOIN entities t ON t.id = p.terminal_entity_id
		WHERE p.terminal_entity_id = ?
	`
	args := []any{entity["id"]}
	if side != "" {
		if side != "buy" && side != "sell" {
			return nil, fmt.Errorf("side must be 'buy' or 'sell'")
		}
		sqlText += " AND p.side = ?"
		args = append(args, side)
	}
	sqlText += " ORDER BY c.canonical_name, p.side"
	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}

func commodityCandidates(matches []map[string]any) []map[string]any {
	seen := map[int64]bool{}
	out := []map[string]any{}
	for _, m := range matches {
		if str(m["kind"]) != "commodity" {
			continue
		}
		id := asInt64(m["id"])
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, m)
	}
	return out
}

func pickEntity(matches []map[string]any, kind string) map[string]any {
	for _, m := range matches {
		if str(m["kind"]) == kind {
			return m
		}
	}
	return nil
}

func asInt64(v any) int64 {
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
