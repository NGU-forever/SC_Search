from __future__ import annotations

import re
import sqlite3
from typing import Any

from sc_search.query.resolve import resolve_name

_TOKEN_SPLIT = re.compile(r"[\s_\-]+")


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


def normalize_mission_text(text: str) -> str:
    """Lowercase and strip spaces/underscores/hyphens for fuzzy mission matching."""
    return re.sub(r"[\s_\-]+", "", (text or "").lower())


def rank_keyword_tokens(rank_keyword: str) -> list[str]:
    """Split rank_keyword into non-empty tokens (whitespace / _ / -)."""
    return [t for t in _TOKEN_SPLIT.split(rank_keyword.strip()) if t]


def _haystacks_for_row(canonical: str, aliases: list[str]) -> list[str]:
    raw = [canonical, *aliases]
    return [normalize_mission_text(x) for x in raw if x]


def _tokens_match(haystacks: list[str], tokens: list[str]) -> bool:
    """AND: every token must appear in at least one normalized haystack."""
    norms = [normalize_mission_text(t) for t in tokens]
    norms = [n for n in norms if n]
    if not norms:
        return True
    for n in norms:
        if not any(n in h for h in haystacks):
            return False
    return True


def _score_match(haystacks: list[str], tokens: list[str], aliases: list[str]) -> float:
    """Higher = better. Prefer more token coverage and shorter title aliases."""
    norms = [normalize_mission_text(t) for t in tokens if normalize_mission_text(t)]
    if not norms:
        return 0.0
    hits = 0
    for n in norms:
        if any(n in h for h in haystacks):
            hits += 1
    shortest_alias = min((len(a) for a in aliases if a), default=len(haystacks[0]) if haystacks else 99)
    return float(hits) * 100.0 - float(shortest_alias)


def list_missions(
    conn: sqlite3.Connection,
    faction: str | None = None,
    rank_keyword: str | None = None,
    system: str | None = None,
    limit: int = 10,
) -> list[dict]:
    sql = """
        SELECT
          e.id AS entity_id,
          e.canonical_name,
          e.game_version,
          fc.value_text AS contractor,
          fr.value_num AS reward_uec,
          fmt.value_text AS mission_type,
          fs.value_text AS systems,
          COALESCE(
            (
              SELECT a.alias
              FROM aliases a
              WHERE a.entity_id = e.id
                AND a.alias != e.canonical_name
              ORDER BY length(a.alias) ASC, a.alias
              LIMIT 1
            ),
            e.canonical_name
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

    if system and system.strip():
        sql += " AND COALESCE(fs.value_text, '') LIKE ? COLLATE NOCASE"
        params.append(f"%{system.strip()}%")

    # Fetch a wider pool when ranking by keyword, then filter/score in Python.
    tokens = rank_keyword_tokens(rank_keyword) if rank_keyword and rank_keyword.strip() else []
    fetch_limit = max(limit * 20, 200) if tokens else limit
    sql += " ORDER BY fr.value_num DESC NULLS LAST LIMIT ?"
    params.append(fetch_limit)

    rows = [dict(r) for r in conn.execute(sql, params).fetchall()]
    if not tokens:
        return [{k: v for k, v in row.items() if k != "entity_id"} for row in rows[:limit]]

    entity_ids = [int(r["entity_id"]) for r in rows]
    alias_map: dict[int, list[str]] = {eid: [] for eid in entity_ids}
    if entity_ids:
        placeholders = ",".join("?" for _ in entity_ids)
        for eid, alias in conn.execute(
            f"SELECT entity_id, alias FROM aliases WHERE entity_id IN ({placeholders})",
            entity_ids,
        ):
            alias_map.setdefault(int(eid), []).append(str(alias))

    scored: list[tuple[float, dict]] = []
    for row in rows:
        eid = int(row["entity_id"])
        aliases = alias_map.get(eid, [])
        canonical = str(row.get("canonical_name") or "")
        haystacks = _haystacks_for_row(canonical, aliases)
        if not _tokens_match(haystacks, tokens):
            continue
        score = _score_match(haystacks, tokens, aliases)
        clean = {k: v for k, v in row.items() if k != "entity_id"}
        scored.append((score, clean))

    scored.sort(key=lambda x: (-x[0], -(x[1].get("reward_uec") or 0)))
    return [row for _, row in scored[:limit]]


def mission_near_misses(
    conn: sqlite3.Connection,
    *,
    faction: str | None = None,
    rank_keyword: str | None = None,
    system: str | None = None,
    limit: int = 3,
) -> list[dict[str, Any]]:
    """Top near matches when list_missions is empty (for meta.near_misses)."""
    tokens = rank_keyword_tokens(rank_keyword) if rank_keyword and rank_keyword.strip() else []
    if not tokens and not (faction and faction.strip()):
        return []

    sql = """
        SELECT
          e.id AS entity_id,
          e.canonical_name,
          COALESCE(
            (
              SELECT a.alias
              FROM aliases a
              WHERE a.entity_id = e.id
                AND a.alias != e.canonical_name
              ORDER BY length(a.alias) ASC, a.alias
              LIMIT 1
            ),
            e.canonical_name
          ) AS title,
          fc.value_text AS contractor,
          fs.value_text AS systems
        FROM entities e
        INNER JOIN facts fc ON fc.entity_id = e.id AND fc.key = 'contractor'
        LEFT JOIN facts fs ON fs.entity_id = e.id AND fs.key = 'systems'
        WHERE e.kind = 'mission'
    """
    params: list[object] = []
    if faction and faction.strip():
        pattern = _contractor_pattern(conn, faction)
        # Broader: any contractor containing a chunk of the faction string
        chunk = pattern[: max(3, min(len(pattern), 12))]
        sql += " AND fc.value_text LIKE ? COLLATE NOCASE"
        params.append(f"%{chunk}%")
    if system and system.strip():
        sql += " AND COALESCE(fs.value_text, '') LIKE ? COLLATE NOCASE"
        params.append(f"%{system.strip()}%")
    sql += " LIMIT 500"

    rows = [dict(r) for r in conn.execute(sql, params).fetchall()]
    if not rows:
        # Fall back: all missions (capped) for keyword-only near misses
        rows = [
            dict(r)
            for r in conn.execute(
                """
                SELECT
                  e.id AS entity_id,
                  e.canonical_name,
                  COALESCE(
                    (
                      SELECT a.alias FROM aliases a
                      WHERE a.entity_id = e.id AND a.alias != e.canonical_name
                      ORDER BY length(a.alias) ASC, a.alias LIMIT 1
                    ),
                    e.canonical_name
                  ) AS title,
                  fc.value_text AS contractor,
                  fs.value_text AS systems
                FROM entities e
                INNER JOIN facts fc ON fc.entity_id = e.id AND fc.key = 'contractor'
                LEFT JOIN facts fs ON fs.entity_id = e.id AND fs.key = 'systems'
                WHERE e.kind = 'mission'
                LIMIT 500
                """
            ).fetchall()
        ]

    entity_ids = [int(r["entity_id"]) for r in rows]
    alias_map: dict[int, list[str]] = {eid: [] for eid in entity_ids}
    if entity_ids:
        placeholders = ",".join("?" for _ in entity_ids)
        for eid, alias in conn.execute(
            f"SELECT entity_id, alias FROM aliases WHERE entity_id IN ({placeholders})",
            entity_ids,
        ):
            alias_map.setdefault(int(eid), []).append(str(alias))

    scored: list[tuple[float, dict[str, Any]]] = []
    for row in rows:
        eid = int(row["entity_id"])
        aliases = alias_map.get(eid, [])
        canonical = str(row.get("canonical_name") or "")
        haystacks = _haystacks_for_row(canonical, aliases)
        if tokens:
            # Soft: count partial token hits (OR scoring) for near misses
            norms = [normalize_mission_text(t) for t in tokens if normalize_mission_text(t)]
            hit = sum(1 for n in norms if any(n in h for h in haystacks))
            if hit == 0:
                continue
            score = float(hit) * 100.0 - float(
                min((len(a) for a in aliases if a), default=len(canonical) or 99)
            )
        else:
            score = 1.0
        scored.append(
            (
                score,
                {
                    "canonical_name": canonical,
                    "title": row.get("title"),
                    "score": round(score, 2),
                },
            )
        )

    scored.sort(key=lambda x: -x[0])
    return [item for _, item in scored[:limit]]
