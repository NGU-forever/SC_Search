from __future__ import annotations

import sqlite3


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
        ORDER BY e.kind, e.canonical_name
        """,
        (q,),
    ).fetchall()
    if rows:
        return [dict(r) for r in rows]

    rows = conn.execute(
        """
        SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               e.canonical_name AS matched_alias, 'en' AS matched_lang
        FROM entities e
        WHERE e.canonical_name = ? COLLATE NOCASE
        ORDER BY e.kind, e.canonical_name
        """,
        (q,),
    ).fetchall()
    if rows:
        return [dict(r) for r in rows]

    like = f"%{q}%"
    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               a.alias AS matched_alias, a.lang AS matched_lang
        FROM aliases a
        JOIN entities e ON e.id = a.entity_id
        WHERE a.alias LIKE ? COLLATE NOCASE
        ORDER BY e.kind, e.canonical_name
        LIMIT 25
        """,
        (like,),
    ).fetchall()
    if rows:
        return [dict(r) for r in rows]

    rows = conn.execute(
        """
        SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               e.canonical_name AS matched_alias, 'en' AS matched_lang
        FROM entities e
        WHERE e.canonical_name LIKE ? COLLATE NOCASE
        ORDER BY e.kind, e.canonical_name
        LIMIT 25
        """,
        (like,),
    ).fetchall()
    return [dict(r) for r in rows]
