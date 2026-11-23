package query

import (
	"database/sql"
	"strings"
)

func SearchDocs(db *sql.DB, queryText string, limit int) ([]map[string]any, error) {
	q := strings.TrimSpace(queryText)
	if q == "" {
		return []map[string]any{}, nil
	}
	if limit <= 0 {
		limit = 5
	}
	ftsQuery := strings.Join(strings.Fields(q), " ")
	rows, err := db.Query(`
		SELECT d.id, d.title, d.url, d.body, d.source, d.game_version, d.entity_id,
		       snippet(documents_fts, 1, '[', ']', '…', 24) AS snippet
		FROM documents_fts
		JOIN documents d ON d.id = documents_fts.rowid
		WHERE documents_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, ftsQuery, limit)
	if err != nil {
		like := "%" + q + "%"
		rows2, err2 := db.Query(`
			SELECT id, title, url, body, source, game_version, entity_id,
			       substr(body, 1, 160) AS snippet
			FROM documents
			WHERE title LIKE ? OR body LIKE ?
			LIMIT ?
		`, like, like, limit)
		if err2 != nil {
			return nil, err2
		}
		defer rows2.Close()
		return scanMaps(rows2)
	}
	defer rows.Close()
	return scanMaps(rows)
}
