from __future__ import annotations

import re
import sqlite3

# Authoritative Live patch for display when SCMDB has not been synced yet.
LIVE_FALLBACK = "4.10.3"

_LIVE_PREFIX = re.compile(r"^(\d+\.\d+(?:\.\d+)?)")


def short_live_label(game_version: str | None) -> str | None:
    """Normalize ``4.10.3-live.13220821`` → ``4.10.3``."""
    if not game_version:
        return None
    text = str(game_version).strip()
    if not text:
        return None
    m = _LIVE_PREFIX.match(text)
    return m.group(1) if m else text


def live_game_version(conn: sqlite3.Connection) -> str:
    """Live patch for answers: prefer latest ok SCMDB run, else ``LIVE_FALLBACK``."""
    rows = data_freshness(conn, source="scmdb")
    for row in rows:
        if row.get("status") == "ok":
            label = short_live_label(row.get("game_version"))
            if label:
                return label
    return LIVE_FALLBACK


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
