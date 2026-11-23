from __future__ import annotations

import sqlite3


def list_locations(
    conn: sqlite3.Connection,
    *,
    query: str | None = None,
    system: str | None = None,
    location_type: str | None = None,
    limit: int = 20,
) -> list[dict]:
    """Search SCMDB location entities (name/type/system; no coordinates)."""
    sql = """
        SELECT
          e.id,
          e.canonical_name,
          e.game_version,
          ft.value_text AS location_type,
          fsys.value_text AS system,
          fp.value_text AS planet,
          fm.value_text AS moon
        FROM entities e
        LEFT JOIN facts ft ON ft.entity_id = e.id AND ft.key = 'location_type'
        LEFT JOIN facts fsys ON fsys.entity_id = e.id AND fsys.key = 'system'
        LEFT JOIN facts fp ON fp.entity_id = e.id AND fp.key = 'planet'
        LEFT JOIN facts fm ON fm.entity_id = e.id AND fm.key = 'moon'
        WHERE e.kind = 'location'
    """
    params: list[object] = []

    if query and query.strip():
        q = f"%{query.strip()}%"
        sql += """
          AND (
            e.canonical_name LIKE ? COLLATE NOCASE
            OR EXISTS (
              SELECT 1 FROM aliases a
              WHERE a.entity_id = e.id AND a.alias LIKE ? COLLATE NOCASE
            )
          )
        """
        params.extend([q, q])

    if system and system.strip():
        sql += " AND COALESCE(fsys.value_text, '') LIKE ? COLLATE NOCASE"
        params.append(f"%{system.strip()}%")

    if location_type and location_type.strip():
        sql += " AND COALESCE(ft.value_text, '') LIKE ? COLLATE NOCASE"
        params.append(f"%{location_type.strip()}%")

    sql += " ORDER BY e.canonical_name ASC LIMIT ?"
    params.append(max(1, min(int(limit), 100)))

    return [dict(r) for r in conn.execute(sql, params).fetchall()]
