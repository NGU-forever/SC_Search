package ingest

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	scmdbMirrorBase = "https://raw.githubusercontent.com/KrovaxCode/SCMDB_DATA/main/data"
	scmdbBase       = "https://scmdb.net/data"
	scmdbUserAgent  = "SC_Search-ingest/0.1"
	scmdbSource     = "scmdb"
)

func SyncScmdbPayload(conn *sql.DB, payload map[string]any) (int, error) {
	runID, err := StartRun(conn, scmdbSource, "")
	if err != nil {
		return 0, err
	}
	gv := firstNonEmpty(strAny(payload["version"]), "unknown")
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyScmdbPayload(tx, payload, runID, gv)
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}

// SyncScmdbLive downloads versions + merged JSON and applies.

func SyncScmdbLive(conn *sql.DB, repoRoot, channel, version string) (int, error) {
	runID, err := StartRun(conn, scmdbSource, "")
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	selected, err := resolveScmdbVersion(client, channel, version)
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	raw, err := scmdbGetJSON(client, strAny(selected["file"]))
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	payload := asStringMap(raw)
	if payload == nil {
		err := fmt.Errorf("%s was not a JSON object", strAny(selected["file"]))
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rawDir := filepath.Join(repoRoot, "data", "raw")
	_ = os.MkdirAll(rawDir, 0o755)
	if b, err := json.Marshal(payload); err == nil {
		_ = os.WriteFile(filepath.Join(rawDir, strAny(selected["file"])), b, 0o644)
	}
	gv := firstNonEmpty(strAny(payload["version"]), strAny(selected["version"]))
	count, err := applyInTx(conn, func(tx *sql.Tx) (int, error) {
		return ApplyScmdbPayload(tx, payload, runID, gv)
	})
	if err != nil {
		_ = FinishRun(conn, runID, "failed", nil, err.Error(), "")
		return 0, err
	}
	rc := count
	if err := FinishRun(conn, runID, "ok", &rc, "", gv); err != nil {
		return count, err
	}
	return count, nil
}
