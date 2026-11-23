package query

import (
	"database/sql"
	"strings"
)

func ListLocations(db *sql.DB, queryText, system, locationType string, limit int) ([]map[string]any, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	sqlText := `
		SELECT
		  e.id,
		  e.canonical_name,
		  e.game_version,
		  ft.value_text AS location_type,
		  fsys.value_text AS system,
		  fp.value_text AS planet,
		  fm.value_text AS moon
		FROM entities e
		LEFT JOIN facts ft ON ft.entity_id = e.id AND ft.key = 'location_type'
		LEFT JOIN facts fsys ON fsys.entity_id = e.id AND fsys.key = 'system'
		LEFT JOIN facts fp ON fp.entity_id = e.id AND fp.key = 'planet'
		LEFT JOIN facts fm ON fm.entity_id = e.id AND fm.key = 'moon'
		WHERE e.kind = 'location'
	`
	args := []any{}
	if q := strings.TrimSpace(queryText); q != "" {
		like := "%" + q + "%"
		sqlText += `
		  AND (
		    e.canonical_name LIKE ? COLLATE NOCASE
		    OR EXISTS (
		      SELECT 1 FROM aliases a
		      WHERE a.entity_id = e.id AND a.alias LIKE ? COLLATE NOCASE
		    )
		  )
		`
		args = append(args, like, like)
	}
	if s := strings.TrimSpace(system); s != "" {
		sqlText += " AND COALESCE(fsys.value_text, '') LIKE ? COLLATE NOCASE"
		args = append(args, "%"+s+"%")
	}
	if lt := strings.TrimSpace(locationType); lt != "" {
		sqlText += " AND COALESCE(ft.value_text, '') LIKE ? COLLATE NOCASE"
		args = append(args, "%"+lt+"%")
	}
	sqlText += " ORDER BY e.canonical_name ASC LIMIT ?"
	args = append(args, limit)
	rows, err := db.Query(sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}
