from __future__ import annotations

import sqlite3

from sc_search.query.resolve import resolve_name


def _pick_entity(matches: list[dict], kind: str) -> dict | None:
    for m in matches:
        if m.get("kind") == kind:
            return m
    return None


def _commodity_candidates(matches: list[dict]) -> list[dict]:
    seen: set[int] = set()
    out: list[dict] = []
    for m in matches:
        if m.get("kind") != "commodity":
            continue
        eid = int(m["id"])
        if eid in seen:
            continue
        seen.add(eid)
        out.append(m)
    return out


def best_prices(
    conn: sqlite3.Connection,
    commodity: str,
    side: str,
    system: str | None = None,
    limit: int = 10,
) -> list[dict]:
    if side not in {"buy", "sell"}:
        raise ValueError("side must be 'buy' or 'sell'")
    matches = resolve_name(conn, commodity)
    candidates = _commodity_candidates(matches)
    if not candidates:
        return []

    order = "ASC" if side == "buy" else "DESC"
    base_sql = f"""
        SELECT
          c.canonical_name AS commodity,
          t.canonical_name AS terminal,
          p.side,
          p.price,
          p.scu_available,
          p.reported_at,
          p.source,
          p.game_version
        FROM prices p
        JOIN entities c ON c.id = p.commodity_entity_id
        JOIN entities t ON t.id = p.terminal_entity_id
        WHERE p.commodity_entity_id = ?
          AND p.side = ?
    """
    # Prefer the spelling/entity that actually has UEX rows (Aluminum vs Aluminium,
    # Quantainium vs Quantanium).
    for entity in candidates:
        sql = base_sql
        params: list[object] = [entity["id"], side]
        if system:
            sql += " AND t.canonical_name LIKE ? COLLATE NOCASE"
            params.append(f"%{system}%")
        sql += f" ORDER BY p.price {order} LIMIT ?"
        params.append(limit)
        rows = [dict(r) for r in conn.execute(sql, params).fetchall()]
        if rows:
            return rows
    return []


def terminal_prices(
    conn: sqlite3.Connection,
    terminal: str,
    side: str | None = None,
) -> list[dict]:
    matches = resolve_name(conn, terminal)
    entity = _pick_entity(matches, "terminal")
    if entity is None:
        return []

    sql = """
        SELECT
          c.canonical_name AS commodity,
          t.canonical_name AS terminal,
          p.side,
          p.price,
          p.scu_available,
          p.reported_at,
          p.source,
          p.game_version
        FROM prices p
        JOIN entities c ON c.id = p.commodity_entity_id
        JOIN entities t ON t.id = p.terminal_entity_id
        WHERE p.terminal_entity_id = ?
    """
    params: list[object] = [entity["id"]]
    if side:
        if side not in {"buy", "sell"}:
            raise ValueError("side must be 'buy' or 'sell'")
        sql += " AND p.side = ?"
        params.append(side)
    sql += " ORDER BY c.canonical_name, p.side"
    return [dict(r) for r in conn.execute(sql, params).fetchall()]
