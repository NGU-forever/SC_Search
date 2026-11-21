from __future__ import annotations

import sqlite3

from sc_search.query.resolve import resolve_name


def _pick_entity(matches: list[dict], kind: str) -> dict | None:
    for m in matches:
        if m.get("kind") == kind:
            return m
    return None


def _contractor_pattern(conn: sqlite3.Connection, faction: str) -> str:
    text = faction.strip()
    matches = resolve_name(conn, text)
    faction_match = _pick_entity(matches, "faction")
    if faction_match:
        return str(faction_match["canonical_name"])
    if matches:
        return str(matches[0]["canonical_name"])
    return text


def list_missions(
    conn: sqlite3.Connection,
    faction: str | None = None,
    rank_keyword: str | None = None,
    system: str | None = None,
    limit: int = 10,
) -> list[dict]:
    sql = """
        SELECT
          e.canonical_name,
          e.game_version,
          fc.value_text AS contractor,
          fr.value_num AS reward_uec,
          fmt.value_text AS mission_type,
          fs.value_text AS systems,
          (
            SELECT a.alias
            FROM aliases a
            WHERE a.entity_id = e.id
            ORDER BY length(a.alias) DESC, a.alias
            LIMIT 1
          ) AS title
        FROM entities e
        INNER JOIN facts fc ON fc.entity_id = e.id AND fc.key = 'contractor'
        LEFT JOIN facts fr ON fr.entity_id = e.id AND fr.key = 'reward_uec'
        LEFT JOIN facts fmt ON fmt.entity_id = e.id AND fmt.key = 'mission_type'
        LEFT JOIN facts fs ON fs.entity_id = e.id AND fs.key = 'systems'
        WHERE e.kind = 'mission'
    """
    params: list[object] = []

    if faction and faction.strip():
        pattern = _contractor_pattern(conn, faction)
        sql += " AND fc.value_text LIKE ? COLLATE NOCASE"
        params.append(f"%{pattern}%")

    if rank_keyword and rank_keyword.strip():
        rk = f"%{rank_keyword.strip()}%"
        sql += """
          AND (
            e.canonical_name LIKE ? COLLATE NOCASE
            OR EXISTS (
              SELECT 1 FROM aliases a
              WHERE a.entity_id = e.id AND a.alias LIKE ? COLLATE NOCASE
            )
          )
        """
        params.extend([rk, rk])

    if system and system.strip():
        sql += " AND COALESCE(fs.value_text, '') LIKE ? COLLATE NOCASE"
        params.append(f"%{system.strip()}%")

    sql += " ORDER BY fr.value_num DESC NULLS LAST LIMIT ?"
    params.append(limit)

    return [dict(r) for r in conn.execute(sql, params).fetchall()]