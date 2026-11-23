package query

import (
	"database/sql"
	"regexp"
	"strings"
)

const LiveFallback = "4.10.3"

var livePrefix = regexp.MustCompile(`^(\d+\.\d+(?:\.\d+)?)`)

func ShortLiveLabel(gameVersion string) string {
	text := strings.TrimSpace(gameVersion)
	if text == "" {
		return ""
	}
	if m := livePrefix.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return text
}

func LiveGameVersion(db *sql.DB) (string, error) {
	rows, err := DataFreshness(db, "scmdb")
	if err != nil {
		return LiveFallback, err
	}
	for _, row := range rows {
		if str(row["status"]) == "ok" {
			if label := ShortLiveLabel(str(row["game_version"])); label != "" {
				return label, nil
			}
		}
	}
	return LiveFallback, nil
}

func DataFreshness(db *sql.DB, source string) ([]map[string]any, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if source != "" {
		rows, err = db.Query(`
			SELECT id, source, game_version, started_at, finished_at, status, row_count, notes
			FROM ingest_runs
			WHERE source = ?
			ORDER BY id DESC
			LIMIT 5
		`, source)
	} else {
		rows, err = db.Query(`
			SELECT r.id, r.source, r.game_version, r.started_at, r.finished_at,
			       r.status, r.row_count, r.notes
			FROM ingest_runs r
			INNER JOIN (
			  SELECT source, MAX(id) AS max_id
			  FROM ingest_runs
			  GROUP BY source
			) latest ON r.id = latest.max_id
			ORDER BY r.source
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return ""
	}
}
