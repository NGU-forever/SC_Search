from __future__ import annotations

import sqlite3
from datetime import datetime, timezone


def _utcnow() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def start_run(conn: sqlite3.Connection, source: str, game_version: str | None = None) -> int:
    cur = conn.execute(
        """
        INSERT INTO ingest_runs (source, game_version, started_at, status)
        VALUES (?, ?, ?, 'running')
        """,
        (source, game_version, _utcnow()),
    )
    return int(cur.lastrowid)


def finish_run(
    conn: sqlite3.Connection,
    run_id: int,
    *,
    status: str,
    row_count: int | None = None,
    notes: str | None = None,
    game_version: str | None = None,
) -> None:
    if game_version is not None:
        conn.execute(
            """
            UPDATE ingest_runs
            SET finished_at = ?, status = ?, row_count = ?, notes = ?, game_version = ?
            WHERE id = ?
            """,
            (_utcnow(), status, row_count, notes, game_version, run_id),
        )
    else:
        conn.execute(
            """
            UPDATE ingest_runs
            SET finished_at = ?, status = ?, row_count = ?, notes = ?
            WHERE id = ?
            """,
            (_utcnow(), status, row_count, notes, run_id),
        )


def upsert_entity(
    conn: sqlite3.Connection,
    *,
    kind: str,
    canonical_name: str,
    game_version: str,
    slug: str | None = None,
    uuid: str | None = None,
) -> int:
    now = _utcnow()
    if uuid:
        row = conn.execute(
            """
            SELECT id FROM entities
            WHERE kind = ? AND uuid = ? AND game_version = ?
            """,
            (kind, uuid, game_version),
        ).fetchone()
    else:
        row = conn.execute(
            """
            SELECT id FROM entities
            WHERE kind = ? AND canonical_name = ? AND game_version = ?
            """,
            (kind, canonical_name, game_version),
        ).fetchone()

    if row:
        entity_id = int(row["id"])
        conn.execute(
            """
            UPDATE entities
            SET canonical_name = ?, slug = ?, uuid = ?, updated_at = ?
            WHERE id = ?
            """,
            (canonical_name, slug, uuid, now, entity_id),
        )
        return entity_id

    cur = conn.execute(
        """
        INSERT INTO entities (kind, canonical_name, slug, uuid, game_version, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
        """,
        (kind, canonical_name, slug, uuid, game_version, now),
    )
    return int(cur.lastrowid)


def upsert_alias(
    conn: sqlite3.Connection,
    entity_id: int,
    alias: str,
    lang: str,
    source: str,
) -> None:
    conn.execute(
        """
        INSERT INTO aliases (entity_id, alias, lang, source)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(entity_id, alias, lang, source) DO NOTHING
        """,
        (entity_id, alias, lang, source),
    )


def replace_price(
    conn: sqlite3.Connection,
    *,
    commodity_id: int,
    terminal_id: int,
    side: str,
    price: float,
    source: str,
    game_version: str,
    ingest_run_id: int,
    scu_available: float | None = None,
    reported_at: str | None = None,
) -> None:
    conn.execute(
        """
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
        """,
        (
            commodity_id,
            terminal_id,
            side,
            price,
            scu_available,
            reported_at,
            source,
            game_version,
            ingest_run_id,
        ),
    )
