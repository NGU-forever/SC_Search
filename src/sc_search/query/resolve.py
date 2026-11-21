from __future__ import annotations

import sqlite3

_KIND_RANK = {
    "ship": 0,
    "location": 1,
    "commodity": 2,
    "terminal": 3,
    "item": 4,
    "faction": 5,
    "blueprint": 6,
    "blueprint_pool": 7,
    "mission": 8,
}


def _rank_rows(rows: list) -> list[dict]:
    items = [dict(r) for r in rows]
    items.sort(
        key=lambda m: (
            _KIND_RANK.get(str(m.get("kind")), 99),
            str(m.get("canonical_name") or ""),
        )
    )
    return items


def resolve_name(conn: sqlite3.Connection, text: str) -> list[dict]:
    q = text.strip()
    if not q:
        return []

    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               a.alias AS matched_alias, a.lang AS matched_lang
        FROM aliases a
        JOIN entities e ON e.id = a.entity_id
        WHERE a.alias = ? COLLATE NOCASE
        """,
        (q,),
    ).fetchall()
    if rows:
        return _rank_rows(rows)

    rows = conn.execute(
        """
        SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               e.canonical_name AS matched_alias, 'en' AS matched_lang
        FROM entities e
        WHERE e.canonical_name = ? COLLATE NOCASE
        """,
        (q,),
    ).fetchall()
    if rows:
        return _rank_rows(rows)

    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               a.alias AS matched_alias, a.lang AS matched_lang
        FROM aliases a
        JOIN entities e ON e.id = a.entity_id
        WHERE (? LIKE '%' || a.alias || '%') COLLATE NOCASE
          AND length(trim(a.alias)) >= 2
        """,
        (q,),
    ).fetchall()
    if rows:
        return _rank_rows(rows)

    rows = conn.execute(
        """
        SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               e.canonical_name AS matched_alias, 'en' AS matched_lang
        FROM entities e
        WHERE (? LIKE '%' || e.canonical_name || '%') COLLATE NOCASE
          AND length(trim(e.canonical_name)) >= 2
        """,
        (q,),
    ).fetchall()
    if rows:
        return _rank_rows(rows)

    like = f"%{q}%"
    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               a.alias AS matched_alias, a.lang AS matched_lang
        FROM aliases a
        JOIN entities e ON e.id = a.entity_id
        WHERE a.alias LIKE ? COLLATE NOCASE
        LIMIT 50
        """,
        (like,),
    ).fetchall()
    if rows:
        return _rank_rows(rows)[:25]

    rows = conn.execute(
        """
        SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               e.canonical_name AS matched_alias, 'en' AS matched_lang
        FROM entities e
        WHERE e.canonical_name LIKE ? COLLATE NOCASE
        LIMIT 50
        """,
        (like,),
    ).fetchall()
    return _rank_rows(rows)[:25]
