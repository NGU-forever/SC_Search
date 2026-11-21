from __future__ import annotations

import re
import sqlite3

from sc_search.query.entities import entity_facts
from sc_search.query.freshness import live_game_version
from sc_search.query.plan import query_plan
from sc_search.query.prices import best_prices, terminal_prices
from sc_search.query.search import search_docs

_OBTAIN_RE = re.compile(
    r"(哪里买|在哪买|能买到|买得到|多少钱|价格|报价|哪[买卖出]|买点|卖点|"
    r"获取|怎么得|如何得|掉落|奖励|蓝图|制造|制作|配方|材料|"
    r"\bbuy\b|\bsell\b|\bprice\b|\bcrafting\b|\bblueprint\b|买)",
    re.I,
)
_CRAFT_MATERIALS_RE = re.compile(r"(材料|配方|制造|制作|crafting)", re.I)

_PLACEHOLDER_VERSIONS = frozenset(
    {"scw-live", "wiki-live", "aliases", "localization-full", "unknown"}
)


def _draft_live_suffix(live: str, source_ver: str | None = None) -> str:
    """Show Live as authority; mention stale source tags (e.g. UEX 3.24.2) separately."""
    if not live:
        return ""
    src = (source_ver or "").strip()
    if (
        src
        and src not in _PLACEHOLDER_VERSIONS
        and src != live
        and not src.startswith(f"{live}-")
        and not src.startswith(f"{live}.")
    ):
        return f"（Live {live}；来源标注 {src}）"
    return f"（Live {live}）"

_CARGO_KEYS = ("cargo_capacity", "cargo", "scu", "货舱")
_FACT_KEY_ZH = {
    "cargo_capacity": "货舱",
    "cargo_scu": "货舱",
    "cargo": "货舱",
    "manufacturer": "制造商",
    "manufacturer_code": "制造商代码",
    "length": "长度",
    "beam": "宽度",
    "height": "高度",
    "mass": "质量",
    "mass_total": "质量",
    "size": "尺寸",
    "type": "类型",
    "career": "职业",
    "foci": "定位",
    "production_status": "生产状态",
    "crew": "乘员",
    "hp": "结构值",
}
_SOFT_VERIFY = "要用浏览器核实吗？"


def _label_fact_key(key: str | None) -> str:
    k = (key or "").strip()
    if not k:
        return "参数"
    return _FACT_KEY_ZH.get(k) or _FACT_KEY_ZH.get(k.lower()) or k


def _slim_fact(f: dict) -> dict:
    key = f.get("key")
    return {
        "key": key,
        "key_zh": _label_fact_key(str(key) if key is not None else None),
        "value_text": f.get("value_text"),
        "value_num": f.get("value_num"),
        "unit": f.get("unit"),
        "source": f.get("source"),
        "game_version": f.get("game_version"),
    }
_ZH_KEY_HINTS = (
    ("厂家", ("manufacturer",)),
    ("制造商", ("manufacturer",)),
    ("长度", ("length", "beam", "height")),
    ("多长", ("length",)),
    ("宽", ("beam",)),
    ("高", ("height",)),
)
_RELATION_LIMIT = 5
_BRIEF_FACT_LIMIT = 5
_FULL_FACT_LIMIT = 25
_BRIEF_PRICE_LIMIT = 3
_FULL_PRICE_LIMIT = 10
_BRIEF_DOC_LIMIT = 2
_FULL_DOC_LIMIT = 5


def _empty_evidence() -> dict:
    return {"entities": [], "facts": [], "relations": [], "prices": [], "docs": [], "missing": []}


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
    missing: list[str] | None = None,
) -> dict:
    return {
        "mode": "blueprint_reward",
        "shop_buyable": "unknown",
        "pool": pool,
        "hint_zh": "本地仅见蓝图/奖励记录，未找到商店报价",
        "missions": list(missions or []),
        "missing": list(missing or []),
    }


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


def _draft_blueprint_unknown(
    label: str,
    pool: str | None,
    live: str,
    src_ver: str | None,
    *,
    missions: list[dict] | None = None,
    wants_materials: bool = False,
    missing: list[str] | None = None,
) -> str:
    pool_bit = f"（池：{pool}）" if pool else ""
    parts = [
        f"{label} 本地仅见蓝图/任务奖励记录{pool_bit}，未找到商店报价；"
        f"不能据此断言绝对买不到"
    ]
    missions = missions or []
    if missions:
        m = missions[0]
        title = m.get("title") or m.get("canonical_name")
        meta = []
        if m.get("contractor"):
            meta.append(str(m["contractor"]))
        if m.get("system"):
            meta.append(str(m["system"]))
        meta_bit = f"（{'，'.join(meta)}）" if meta else ""
        parts.append(f"任务线索：{title}{meta_bit}")
    miss = set(missing or [])
    if wants_materials and "crafting_recipe" in miss:
        parts.append("库未收录制造配方/材料来源，不能编造")
    return "；".join(parts) + f"{_draft_live_suffix(live, src_ver)}。"


def _display_name(entities: list[dict]) -> str:
    if not entities:
        return ""
    e = entities[0]
    zh = e.get("zh")
    name = e["canonical_name"]
    if zh:
        return f"{zh}（{name}）"
    return str(name)


def _fact_value(f: dict) -> str:
    if f.get("value_num") is not None:
        unit = f.get("unit") or ""
        num = f["value_num"]
        if isinstance(num, float) and num.is_integer():
            num = int(num)
        return f"{num}{(' ' + unit) if unit else ''}".strip()
    return str(f.get("value_text") or "")


def _question_wants_cargo(question: str) -> bool:
    q = question or ""
    ql = q.lower()
    return any(
        tok in q or tok in ql for tok in ("货舱", "载货", "cargo", "scu")
    )


def _terminal_from_context(entities: list[dict], question: str) -> str | None:
    terminal = next((e for e in entities if e.get("kind") == "terminal"), None)
    if terminal:
        return str(terminal["canonical_name"])
    q = question or ""
    if ("在某" in q or "在" in q) and entities:
        terminal = next((e for e in entities if e.get("kind") == "terminal"), None)
        if terminal:
            return str(terminal["canonical_name"])
    return None


def _slim_relations(rows: list[dict], limit: int = _RELATION_LIMIT) -> list[dict]:
    return [
        {
            "rel_type": r.get("rel_type"),
            "from_name": r.get("from_name"),
            "to_name": r.get("to_name"),
            "qty": r.get("qty"),
        }
        for r in rows[:limit]
    ]


def _pick_facts(facts: list[dict], question: str, limit: int) -> tuple[list[dict], bool]:
    q = question or ""
    ql = q.lower()
    wants_cargo = _question_wants_cargo(q)
    scored: list[tuple[int, dict]] = []
    for f in facts:
        key = str(f.get("key") or "").lower()
        score = 0
        if wants_cargo and (any(k in key for k in _CARGO_KEYS) or "cargo" in key):
            score += 10
            if "货舱" in q or "cargo" in ql:
                score += 3
        for zh, keys in _ZH_KEY_HINTS:
            if zh in q and any(k in key for k in keys):
                score += 8
        if key in ql or any(tok and tok in key for tok in ql.split() if len(tok) > 1):
            score += 3
        if score:
            scored.append((score, f))
    scored.sort(key=lambda x: -x[0])
    picked: list[dict] = []
    seen: set[int] = set()
    for _, f in scored:
        if len(picked) >= limit:
            break
        fid = id(f)
        if fid not in seen:
            picked.append(f)
            seen.add(fid)
    had_scored_hit = bool(picked)
    for f in facts:
        if len(picked) >= limit:
            break
        fid = id(f)
        if fid not in seen:
            picked.append(f)
            seen.add(fid)
    return [_slim_fact(f) for f in picked], had_scored_hit


def _recompute_confidence(
    intent: str,
    *,
    plan_confidence: str,
    had_scored_hit: bool,
    has_entity: bool,
    facts: list,
    prices: list,
    docs: list,
    missing: list[str],
) -> str:
    _ = missing  # reserved for future gap hints
    if intent == "facts":
        if had_scored_hit and facts:
            return "high"
        if has_entity and facts:
            return "medium"  # backfill only
        if has_entity and not facts:
            return "low"
        return "low"
    if intent == "prices":
        return "high" if prices else "low"
    if intent == "docs":
        return "medium" if docs else "low"
    if intent in {"loadout_calc", "mining_calc"}:
        return "medium" if facts else "low"
    if intent == "live_web":
        return plan_confidence
    return plan_confidence if has_entity else "low"


def _needs_soft_verify(
    intent: str,
    confidence: str,
    *,
    had_scored_hit: bool,
    facts: list,
    prices: list,
    docs: list,
) -> bool:
    # Broader than spec §2.1 low-only: weak facts/prices/docs without scored match.
    if confidence not in {"low", "medium"}:
        return False
    if intent == "facts":
        return not had_scored_hit
    if intent == "prices":
        return not prices
    if intent == "docs":
        return not docs
    return False


def _slim_prices(rows: list[dict]) -> list[dict]:
    return [
        {
            "commodity": r.get("commodity"),
            "terminal": r.get("terminal"),
            "side": r.get("side"),
            "price": r.get("price"),
            "game_version": r.get("game_version"),
            "source": r.get("source"),
        }
        for r in rows
    ]


def answer_question(
    conn: sqlite3.Connection,
    question: str,
    detail_level: str = "brief",
    hint_entity: str | None = None,
) -> dict:
    level = (detail_level or "brief").strip().lower()
    if level not in {"brief", "full"}:
        level = "brief"

    plan = query_plan(conn, question, hint_entity=hint_entity)
    intent = plan["intent"]
    evidence = _empty_evidence()
    evidence["entities"] = list(plan.get("entities") or [])
    follow_ups: list[str] = []
    detail_available = False
    draft = ""
    missing: list[str] = []
    had_scored_hit = False

    fact_limit = _BRIEF_FACT_LIMIT if level == "brief" else _FULL_FACT_LIMIT
    price_limit = _BRIEF_PRICE_LIMIT if level == "brief" else _FULL_PRICE_LIMIT
    doc_limit = _BRIEF_DOC_LIMIT if level == "brief" else _FULL_DOC_LIMIT

    primary = evidence["entities"][0]["canonical_name"] if evidence["entities"] else ""
    label = _display_name(evidence["entities"])
    live = live_game_version(conn)
    primary_kind = (
        evidence["entities"][0].get("kind") if evidence["entities"] else None
    )

    if _wants_obtain_or_price(question) and primary_kind == "blueprint" and primary:
        bundle = entity_facts(conn, primary) or {}
        all_facts = bundle.get("facts") or []
        pool = _pool_name_from_facts(all_facts)
        missions = _missions_for_pool(conn, pool)
        wants_materials = bool(_CRAFT_MATERIALS_RE.search(question or ""))
        acq_missing: list[str] = []
        if wants_materials:
            acq_missing.append("crafting_recipe")
        evidence["acquisition"] = _acquisition_unknown(
            pool, missions=missions, missing=acq_missing
        )
        evidence["facts"] = [_slim_fact(f) for f in all_facts[:fact_limit]]
        evidence["missing"] = list(acq_missing)
        if missions:
            evidence["relations"] = [
                {
                    "rel_type": "mission_reward_blueprint_pool",
                    "from_name": m.get("canonical_name"),
                    "to_name": pool,
                    "qty": None,
                }
                for m in missions[:_RELATION_LIMIT]
            ]
        src_ver = (bundle.get("entity") or {}).get("game_version")
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": "high",
            "live_game_version": live,
            "answer_draft": _draft_blueprint_unknown(
                label or primary,
                pool,
                live,
                src_ver,
                missions=missions,
                wants_materials=wants_materials,
                missing=acq_missing,
            ),
            "evidence": evidence,
            "detail_available": False,
            "follow_ups": [],
            "browser": {"needed": False},
            "plan": plan,
        }

    if intent in {"facts", "loadout_calc", "mining_calc"} and primary:
        bundle = entity_facts(conn, primary)
        if bundle:
            all_facts = bundle.get("facts") or []
            all_relations = bundle.get("relations") or []
            picked, had_scored_hit = _pick_facts(all_facts, question, fact_limit)
            evidence["facts"] = picked
            if level == "full" and all_relations:
                slim = _slim_relations(all_relations)
                evidence["relations"] = slim
            detail_available = len(all_facts) > len(picked) or len(all_relations) > len(
                evidence.get("relations") or []
            )
            src_ver = (bundle.get("entity") or {}).get("game_version")
            if intent == "facts":
                if had_scored_hit and picked:
                    top = picked[0]
                    key_label = top.get("key_zh") or _label_fact_key(
                        str(top.get("key") or "")
                    )
                    draft = (
                        f"{label} 的 {key_label} 为 {_fact_value(top)}"
                        f"{_draft_live_suffix(live, src_ver)}。"
                    )
                    if detail_available:
                        draft += "需要更细的硬点/部件可以说一声。"
                        follow_ups = ["展开全部参数", "只要货舱"]
                else:
                    draft = f"本地库有 {label}，但未找到匹配字段。"
                    missing.append("matching_facts")
            else:
                site = "Erkul" if intent == "loadout_calc" else "Regolith"
                static = ""
                if picked:
                    static = f"静态参考：{picked[0].get('key')}={_fact_value(picked[0])}。"
                draft = (
                    f"{label} 配装/计算需 {site}。"
                    f"{static}"
                    "本地只能给静态参数；要 DPS/配装结果再说，我再按指引开工具站。"
                )
                follow_ups = [f"打开 {site} 查配装", "只要静态参数"]
                detail_available = True
        else:
            missing.append("entity_facts")
            draft = f"未找到 {label or primary} 的本地事实。"

    elif intent == "prices":
        side = "sell"
        for step in plan.get("mcp_steps") or []:
            if step.get("tool") == "sc_best_prices":
                side = (step.get("args") or {}).get("side") or "sell"
        entities = evidence["entities"]
        terminal_name = _terminal_from_context(entities, question)
        commodity_entity = next((e for e in entities if e.get("kind") == "commodity"), None)
        commodity = (commodity_entity or {}).get("canonical_name") or primary or question

        if terminal_name:
            all_rows = terminal_prices(conn, terminal_name, side=side)
            if commodity_entity:
                cn = str(commodity_entity["canonical_name"])
                all_rows = [
                    r
                    for r in all_rows
                    if str(r.get("commodity") or "").lower() == cn.lower()
                ]
            prices_remain = len(all_rows) > price_limit
            rows = all_rows[:price_limit]
        else:
            fetched = best_prices(conn, commodity, side=side, limit=price_limit + 1)
            prices_remain = len(fetched) > price_limit
            rows = fetched[:price_limit]
        evidence["prices"] = _slim_prices(rows)
        if rows:
            top = rows[0]
            ver_suffix = _draft_live_suffix(live, top.get("game_version"))
            if terminal_name and commodity_entity is None:
                draft = (
                    f"{terminal_name} {side} 报价示例：{top.get('commodity')} "
                    f"约 {top.get('price')} aUEC{ver_suffix}。"
                )
            else:
                draft = (
                    f"{top.get('commodity')} {side} 较优：{top.get('terminal')} "
                    f"约 {top.get('price')} aUEC{ver_suffix}。"
                )
            detail_available = prices_remain
            if prices_remain:
                follow_ups = (
                    ["看更多终端", "查买入价"]
                    if side == "sell"
                    else ["看更多终端", "查卖出价"]
                )
        else:
            missing.append("prices")
            target = terminal_name or commodity
            draft = f"本地没有 {target} 的 {side} 报价。"
            detail_available = False

    elif intent == "docs":
        rows = search_docs(conn, question, limit=doc_limit)
        evidence["docs"] = [
            {
                "title": r.get("title"),
                "snippet": (r.get("snippet") or (r.get("body") or ""))[:240],
            }
            for r in rows
        ]
        if rows:
            snip = evidence["docs"][0].get("snippet") or ""
            draft = f"资料摘要：{snip[:120]}…" if snip else "找到相关文档。"
            detail_available = True
            follow_ups = ["展开更多 Wiki 摘要"]
        else:
            missing.append("docs")
            draft = "本地 Wiki 摘要未命中。"
            detail_available = False

    elif intent == "live_web":
        draft = "这类信息需要官网或 Spectrum；本地库不覆盖实时 Patch。需要的话再开浏览器。"
        detail_available = False
        follow_ups = ["打开 RSI 查 Patch Notes"]

    else:
        if not primary:
            draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
            missing.append("entity")
            detail_available = False
        elif not draft:
            draft = "暂无法根据本地数据作答。"
            detail_available = False

    evidence["missing"] = missing
    if not draft:
        draft = "本地知识库未覆盖该问题；不会编造数据。"

    has_entity = bool(evidence["entities"])
    confidence = _recompute_confidence(
        intent,
        plan_confidence=plan["confidence"],
        had_scored_hit=had_scored_hit,
        has_entity=has_entity,
        facts=evidence["facts"],
        prices=evidence["prices"],
        docs=evidence["docs"],
        missing=missing,
    )
    browser = dict(plan.get("browser") or {"needed": False})
    soft = _needs_soft_verify(
        intent,
        confidence,
        had_scored_hit=had_scored_hit,
        facts=evidence["facts"],
        prices=evidence["prices"],
        docs=evidence["docs"],
    )
    if soft:
        if _SOFT_VERIFY not in follow_ups:
            follow_ups.append(_SOFT_VERIFY)
        if intent in {"facts", "prices", "docs"}:
            browser = {**browser, "needed": False}

    return {
        "intent": intent,
        "path": plan["path"],
        "confidence": confidence,
        "live_game_version": live,
        "answer_draft": draft,
        "evidence": evidence,
        "detail_available": detail_available,
        "follow_ups": follow_ups,
        "browser": browser,
        "plan": plan,
    }