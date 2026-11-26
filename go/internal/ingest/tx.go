package ingest

import (
	"database/sql"
	"fmt"
)

// DBTX is satisfied by *sql.DB and *sql.Tx so write helpers work in or out of a transaction.
type DBTX interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// WithTx runs fn inside a single write transaction: commit on nil, rollback on error or panic.
func WithTx(conn *sql.DB, fn func(tx *sql.Tx) error) (err error) {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("%w (rollback: %v)", err, rbErr)
		}
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// applyInTx runs apply inside WithTx and returns its row count; on error nothing is persisted.
func applyInTx(conn *sql.DB, apply func(tx *sql.Tx) (int, error)) (int, error) {
	var count int
	err := WithTx(conn, func(tx *sql.Tx) error {
		n, err := apply(tx)
		if err != nil {
			return err
		}
		count = n
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
