package ingest

import (
	"database/sql"
	"fmt"
	"time"
)

func utcNow() string {
	return time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
}

func StartRun(conn DBTX, source string, gameVersion string) (int64, error) {
	res, err := conn.Exec(`
		INSERT INTO ingest_runs (source, game_version, started_at, status)
		VALUES (?, ?, ?, 'running')
	`, source, nullStr(gameVersion), utcNow())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func FinishRun(conn DBTX, runID int64, status string, rowCount *int, notes, gameVersion string) error {
	if gameVersion != "" {
		_, err := conn.Exec(`
			UPDATE ingest_runs
			SET finished_at = ?, status = ?, row_count = ?, notes = ?, game_version = ?
			WHERE id = ?
		`, utcNow(), status, rowCount, nullStr(notes), gameVersion, runID)
		return err
	}
	_, err := conn.Exec(`
		UPDATE ingest_runs
		SET finished_at = ?, status = ?, row_count = ?, notes = ?
		WHERE id = ?
	`, utcNow(), status, rowCount, nullStr(notes), runID)
	return err
}

func UpsertEntity(conn DBTX, kind, canonicalName, gameVersion, slug, uuid string) (int64, error) {
	now := utcNow()
	var id sql.NullInt64
	if uuid != "" {
		err := conn.QueryRow(`
			SELECT id FROM entities
			WHERE kind = ? AND uuid = ? AND game_version = ?
		`, kind, uuid, gameVersion).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
	}
	if !id.Valid {
		err := conn.QueryRow(`
			SELECT id FROM entities
			WHERE kind = ? AND canonical_name = ? AND game_version = ?
		`, kind, canonicalName, gameVersion).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
	}
	if id.Valid {
		_, err := conn.Exec(`
			UPDATE entities
			SET canonical_name = ?, slug = COALESCE(?, slug),
			    uuid = COALESCE(?, uuid), updated_at = ?
			WHERE id = ?
		`, canonicalName, nullStr(slug), nullStr(uuid), now, id.Int64)
		return id.Int64, err
	}
	res, err := conn.Exec(`
		INSERT INTO entities (kind, canonical_name, slug, uuid, game_version, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, kind, canonicalName, nullStr(slug), nullStr(uuid), gameVersion, now)
	if err == nil {
		return res.LastInsertId()
	}
	// IntegrityError path: re-select
	if uuid != "" {
		err2 := conn.QueryRow(`
			SELECT id FROM entities
			WHERE kind = ? AND uuid = ? AND game_version = ?
		`, kind, uuid, gameVersion).Scan(&id)
		if err2 == nil && id.Valid {
			_, _ = conn.Exec(`
				UPDATE entities
				SET slug = COALESCE(?, slug), uuid = COALESCE(?, uuid), updated_at = ?
				WHERE id = ?
			`, nullStr(slug), nullStr(uuid), now, id.Int64)
			return id.Int64, nil
		}
	}
	err2 := conn.QueryRow(`
		SELECT id FROM entities
		WHERE kind = ? AND canonical_name = ? AND game_version = ?
	`, kind, canonicalName, gameVersion).Scan(&id)
	if err2 == nil && id.Valid {
		_, _ = conn.Exec(`
			UPDATE entities
			SET slug = COALESCE(?, slug), uuid = COALESCE(?, uuid), updated_at = ?
			WHERE id = ?
		`, nullStr(slug), nullStr(uuid), now, id.Int64)
		return id.Int64, nil
	}
	return 0, fmt.Errorf("upsert entity: %w", err)
}

func UpsertAlias(conn DBTX, entityID int64, alias, lang, source string) error {
	_, err := conn.Exec(`
		INSERT INTO aliases (entity_id, alias, lang, source)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(entity_id, alias, lang, source) DO NOTHING
	`, entityID, alias, lang, source)
	return err
}

func ClearSourceFactsRelations(conn DBTX, source, gameVersion string) error {
	if _, err := conn.Exec(`DELETE FROM facts WHERE source = ? AND game_version = ?`, source, gameVersion); err != nil {
		return err
	}
	_, err := conn.Exec(`DELETE FROM relations WHERE source = ? AND game_version = ?`, source, gameVersion)
	return err
}

func InsertRelation(conn DBTX, fromID, toID int64, relType, source, gameVersion string, qty *float64, metaJSON string) error {
	var q any
	if qty != nil {
		q = *qty
	}
	_, err := conn.Exec(`
		INSERT INTO relations (from_id, to_id, rel_type, qty, meta_json, source, game_version)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, fromID, toID, relType, q, nullStr(metaJSON), source, gameVersion)
	return err
}

func ReplacePrice(conn DBTX, commodityID, terminalID int64, side string, price float64, source, gameVersion string, ingestRunID int64, scuAvailable *float64, reportedAt string) error {
	var scu any
	if scuAvailable != nil {
		scu = *scuAvailable
	}
	_, err := conn.Exec(`
		INSERT INTO prices (
		  commodity_entity_id, terminal_entity_id, side, price, scu_available,
		  reported_at, source, game_version, ingest_run_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(commodity_entity_id, terminal_entity_id, side, source, game_version)
		DO UPDATE SET
		  price = excluded.price,
		  scu_available = excluded.scu_available,
		  reported_at = excluded.reported_at,
		  ingest_run_id = excluded.ingest_run_id
	`, commodityID, terminalID, side, price, scu, nullStr(reportedAt), source, gameVersion, ingestRunID)
	return err
}

func UpsertFact(conn DBTX, entityID int64, key, source, gameVersion string, ingestRunID *int64, valueText string, valueNum *float64, unit string) error {
	if _, err := conn.Exec(`
		DELETE FROM facts
		WHERE entity_id = ? AND key = ? AND source = ? AND game_version = ?
	`, entityID, key, source, gameVersion); err != nil {
		return err
	}
	var run any
	if ingestRunID != nil {
		run = *ingestRunID
	}
	var num any
	if valueNum != nil {
		num = *valueNum
	}
	_, err := conn.Exec(`
		INSERT INTO facts (
		  entity_id, key, value_text, value_num, unit, source, game_version, ingest_run_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, entityID, key, nullStr(valueText), num, nullStr(unit), source, gameVersion, run)
	return err
}

func ClearSourceDocuments(conn DBTX, source, gameVersion string) error {
	_, err := conn.Exec(
		`DELETE FROM documents WHERE source = ? AND game_version = ?`,
		source, gameVersion,
	)
	return err
}

func InsertDocument(conn DBTX, title, body, source, gameVersion string, ingestRunID int64, entityID *int64, url string) (int64, error) {
	var eid any
	if entityID != nil {
		eid = *entityID
	}
	res, err := conn.Exec(`
		INSERT INTO documents (entity_id, title, url, body, source, game_version, ingest_run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, eid, title, nullStr(url), body, source, gameVersion, ingestRunID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
