from __future__ import annotations

import sqlite3


def data_freshness(conn: sqlite3.Connection, source: str | None = None) -> list[dict]:
    if source:
        rows = conn.execute(
            """
            SELECT id, source, game_version, started_at, finished_at, status, row_count, notes
            FROM ingest_runs
            WHERE source = ?
            ORDER BY id DESC
            LIMIT 5
            """,
            (source,),
        ).fetchall()
    else:
        rows = conn.execute(
            """
            SELECT r.id, r.source, r.game_version, r.started_at, r.finished_at,
                   r.status, r.row_count, r.notes
            FROM ingest_runs r
            INNER JOIN (
              SELECT source, MAX(id) AS max_id
              FROM ingest_runs
              GROUP BY source
            ) latest ON r.id = latest.max_id
            ORDER BY r.source
            """
        ).fetchall()
    return [dict(r) for r in rows]
