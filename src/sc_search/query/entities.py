from __future__ import annotations

import json
import sqlite3

from sc_search.query.resolve import resolve_name


_COMPACT_FACT_KEYS = frozenset(
    {
        "reward_uec",
        "cost_uec",
        "cooldown",
        "abandoned_cooldown",
        "lifetime",
        "reputation_min",
        "reputation_rank",
        "reputation_max",
        "reputation_rank_max",
        "pyro_regions",
        "site_pool",
        "enemy_min",
        "enemy_max",
        "mission_type",
        "category",
        "contractor",
        "systems",
        "illegal",
        "cargo_capacity",
        "manufacturer",
        "size",
        "location_type",
        "system",
        "planet",
        "moon",
    }
)


def entity_facts(
    conn: sqlite3.Connection,
    name_or_id: str,
    *,
    detail: str = "full",
) -> dict | None:
    text = name_or_id.strip()
    if not text:
        return None

    entity: dict | None = None
    if text.isdigit():
        row = conn.execute(
            """
            SELECT id, kind, canonical_name, slug, uuid, game_version
            FROM entities WHERE id = ?
            """,
            (int(text),),
        ).fetchone()
        if row:
            entity = dict(row)
    if entity is None:
        matches = resolve_name(conn, text)
        if not matches:
            return None
        # Prefer mission/blueprint over others when ambiguous
        priority = {
            "ship": 0,
            "mission": 1,
            "blueprint": 2,
            "blueprint_pool": 3,
            "commodity": 4,
            "item": 5,
            "faction": 6,
        }
        matches = sorted(matches, key=lambda m: priority.get(str(m.get("kind")), 99))
        entity = {
            "id": matches[0]["id"],
            "kind": matches[0]["kind"],
            "canonical_name": matches[0]["canonical_name"],
            "slug": matches[0].get("slug"),
            "uuid": matches[0].get("uuid"),
            "game_version": matches[0].get("game_version"),
        }

    entity_id = int(entity["id"])
    facts = [
        dict(r)
        for r in conn.execute(
            """
            SELECT key, value_text, value_num, unit, source, game_version
            FROM facts WHERE entity_id = ?
            ORDER BY key
            """,
            (entity_id,),
        ).fetchall()
    ]
    aliases = [
        dict(r)
        for r in conn.execute(
            """
            SELECT alias, lang, source FROM aliases WHERE entity_id = ?
            ORDER BY alias
            """,
            (entity_id,),
        ).fetchall()
    ]
    relations_out: list[dict] = []
    for r in conn.execute(
        """
        SELECT rel.id, rel.rel_type, rel.qty, rel.meta_json, rel.source, rel.game_version,
               rel.from_id, rel.to_id,
               fe.canonical_name AS from_name, fe.kind AS from_kind,
               te.canonical_name AS to_name, te.kind AS to_kind
        FROM relations rel
        JOIN entities fe ON fe.id = rel.from_id
        JOIN entities te ON te.id = rel.to_id
        WHERE rel.from_id = ? OR rel.to_id = ?
        ORDER BY rel.rel_type, te.canonical_name
        """,
        (entity_id, entity_id),
    ).fetchall():
        item = dict(r)
        meta = item.pop("meta_json", None)
        if meta:
            try:
                item["meta"] = json.loads(meta)
            except json.JSONDecodeError:
                item["meta"] = meta
        relations_out.append(item)

    mode = (detail or "full").strip().lower()
    if mode == "compact":
        facts = [f for f in facts if f.get("key") in _COMPACT_FACT_KEYS] or facts[:20]
        aliases = aliases[:8]
        relations_out = relations_out[:20]

    return {
        "entity": entity,
        "aliases": aliases,
        "facts": facts,
        "relations": relations_out,
        "detail": mode if mode in {"compact", "full"} else "full",
    }
