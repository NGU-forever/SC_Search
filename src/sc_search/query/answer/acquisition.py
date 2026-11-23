from __future__ import annotations

import json
import sqlite3

from sc_search.query.answer.constants import _OBTAIN_RE

def _wants_obtain_or_price(question: str) -> bool:
    return bool(_OBTAIN_RE.search(question or ""))

def _pool_name_from_facts(facts: list[dict]) -> str | None:
    for f in facts:
        if str(f.get("key") or "") == "pool_name":
            v = f.get("value_text")
            return str(v) if v else None
    return None

def _acquisition_unknown(
    pool: str | None,
    *,
    missions: list[dict] | None = None,
    materials: list[dict] | None = None,
    missing: list[str] | None = None,
    mode: str = "blueprint_reward",
    hint_zh: str | None = None,
) -> dict:
    return {
        "mode": mode,
        "shop_buyable": "unknown",
        "pool": pool,
        "hint_zh": hint_zh or "本地仅见蓝图/奖励记录，未找到商店报价",
        "missions": list(missions or []),
        "materials": list(materials or []),
        "missing": list(missing or []),
    }

def _acquisition_shop_yes(*, shops: list[dict], hint_zh: str | None = None) -> dict:
    return {
        "mode": "shop_buy",
        "shop_buyable": "yes",
        "pool": None,
        "hint_zh": hint_zh or "本地有游戏内商店/aUEC 报价",
        "missions": [],
        "materials": [],
        "shops": list(shops),
        "missing": [],
    }

def _ship_buy_prices(conn: sqlite3.Connection, entity_id: int, *, limit: int = 8) -> list[dict]:
    rows = conn.execute(
        """
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
          AND p.side = 'buy'
          AND p.price > 0
        ORDER BY p.price ASC, t.canonical_name ASC
        LIMIT ?
        """,
        (entity_id, limit),
    ).fetchall()
    return [dict(r) for r in rows]

def _recipe_materials(conn: sqlite3.Connection, blueprint_entity_id: int) -> list[dict]:
    rows = conn.execute(
        """
        SELECT te.canonical_name AS name, rel.qty, rel.meta_json
        FROM relations rel
        JOIN entities te ON te.id = rel.to_id
        WHERE rel.from_id = ?
          AND rel.rel_type = 'blueprint_requires'
        ORDER BY te.canonical_name
        """,
        (blueprint_entity_id,),
    ).fetchall()
    out: list[dict] = []
    for r in rows:
        unit = "SCU"
        ingredient_kind = "resource"
        raw = r["meta_json"]
        if raw:
            try:
                meta = json.loads(raw) if isinstance(raw, str) else raw
                if isinstance(meta, dict):
                    unit = str(meta.get("unit") or unit)
                    ingredient_kind = str(meta.get("ingredient_kind") or ingredient_kind)
            except json.JSONDecodeError:
                pass
        qty = r["qty"]
        out.append(
            {
                "name": r["name"],
                "qty": qty,
                "unit": unit,
                "ingredient_kind": ingredient_kind,
            }
        )
    return out

def _missions_for_pool(conn: sqlite3.Connection, pool: str | None) -> list[dict]:
    if not pool:
        return []
    rows = conn.execute(
        """
        SELECT m.id, m.canonical_name
        FROM entities pool
        JOIN relations rel
          ON rel.to_id = pool.id
         AND rel.rel_type = 'mission_reward_blueprint_pool'
        JOIN entities m ON m.id = rel.from_id AND m.kind = 'mission'
        WHERE pool.canonical_name = ? COLLATE NOCASE
        LIMIT 5
        """,
        (pool,),
    ).fetchall()
    out: list[dict] = []
    for r in rows:
        mid = int(r["id"])
        aliases = conn.execute(
            """
            SELECT alias, lang FROM aliases WHERE entity_id = ?
            ORDER BY CASE lang WHEN 'zh' THEN 0 WHEN 'en' THEN 1 ELSE 2 END, length(alias) DESC
            """,
            (mid,),
        ).fetchall()
        title = None
        for a in aliases:
            alias = str(a["alias"] or "")
            if a["lang"] == "zh" and alias:
                title = alias
                break
        if not title:
            for a in aliases:
                alias = str(a["alias"] or "")
                if alias and alias != r["canonical_name"]:
                    title = alias
                    break
        fact_rows = conn.execute(
            """
            SELECT key, value_text FROM facts
            WHERE entity_id = ? AND key IN ('contractor', 'systems')
            """,
            (mid,),
        ).fetchall()
        fmap = {str(f["key"]): f["value_text"] for f in fact_rows}
        out.append(
            {
                "canonical_name": r["canonical_name"],
                "title": title or r["canonical_name"],
                "contractor": fmap.get("contractor"),
                "system": fmap.get("systems"),
            }
        )
    return out
