package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens the knowledge SQLite DB (SC_SEARCH_DB or <repo>/data/sc_knowledge.sqlite).
func Open(repoRoot string) (*sql.DB, error) {
	path := os.Getenv("SC_SEARCH_DB")
	if path == "" {
		if repoRoot == "" {
			return nil, fmt.Errorf("db: empty repoRoot and no SC_SEARCH_DB")
		}
		path = filepath.Join(repoRoot, "data", "sc_knowledge.sqlite")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("database missing: %s; run Python ingest first", path)
	}
	// modernc driver name is "sqlite"
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}
