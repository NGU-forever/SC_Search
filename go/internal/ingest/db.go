package ingest

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Open creates/opens the knowledge DB (creates parent dirs).
func Open(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// InitDB applies the embedded schema (tables + indexes; idempotent, upgrades existing DBs).
func InitDB(conn *sql.DB) error {
	if _, err := conn.Exec(schemaSQL); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	// Refresh planner stats after (re)creating indexes; cheap on a ~16MB DB.
	if _, err := conn.Exec("PRAGMA optimize"); err != nil {
		return fmt.Errorf("pragma optimize: %w", err)
	}
	return nil
}

// DefaultDBPath returns <repo>/data/sc_knowledge.sqlite or SC_SEARCH_DB.
func DefaultDBPath(repoRoot string) string {
	if override := os.Getenv("SC_SEARCH_DB"); override != "" {
		return override
	}
	return filepath.Join(repoRoot, "data", "sc_knowledge.sqlite")
}

// RecentRuns returns the latest ingest_runs rows.
func RecentRuns(conn *sql.DB, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := conn.Query(`
		SELECT id, source, game_version, started_at, finished_at, status, row_count, notes
		FROM ingest_runs
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			switch t := raw[i].(type) {
			case []byte:
				m[c] = string(t)
			default:
				m[c] = t
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
