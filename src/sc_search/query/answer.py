from __future__ import annotations

import sqlite3

from sc_search.query.entities import entity_facts
from sc_search.query.plan import query_plan
from sc_search.query.prices import best_prices
from sc_search.query.search import search_docs

_CARGO_KEYS = ("cargo_capacity", "cargo", "scu", "货舱")
_BRIEF_FACT_LIMIT = 5
_FULL_FACT_LIMIT = 25
_BRIEF_PRICE_LIMIT = 3
_FULL_PRICE_LIMIT = 10
_BRIEF_DOC_LIMIT = 2
_FULL_DOC_LIMIT = 5


def _empty_evidence() -> dict:
    return {"entities": [], "facts": [], "prices": [], "docs": [], "missing": []}


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


def _pick_facts(facts: list[dict], question: str, limit: int) -> list[dict]:
    q = (question or "").lower()
    scored: list[tuple[int, dict]] = []
    for f in facts:
        key = str(f.get("key") or "").lower()
        score = 0
        if any(k in key or k in q for k in _CARGO_KEYS) and (
            "cargo" in key or "货舱" in q or "cargo" in q
        ):
            score += 10
        if key in q or any(tok and tok in key for tok in q.split() if len(tok) > 1):
            score += 3
        if score:
            scored.append((score, f))
    scored.sort(key=lambda x: -x[0])
    picked = [f for _, f in scored[:limit]]
    if not picked:
        picked = facts[:limit]
    return [
        {
            "key": f.get("key"),
            "value_text": f.get("value_text"),
            "value_num": f.get("value_num"),
            "unit": f.get("unit"),
            "source": f.get("source"),
            "game_version": f.get("game_version"),
        }
        for f in picked
    ]


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

    fact_limit = _BRIEF_FACT_LIMIT if level == "brief" else _FULL_FACT_LIMIT
    price_limit = _BRIEF_PRICE_LIMIT if level == "brief" else _FULL_PRICE_LIMIT
    doc_limit = _BRIEF_DOC_LIMIT if level == "brief" else _FULL_DOC_LIMIT

    primary = evidence["entities"][0]["canonical_name"] if evidence["entities"] else ""
    label = _display_name(evidence["entities"])

    if intent in {"facts", "loadout_calc", "mining_calc"} and primary:
        bundle = entity_facts(conn, primary)
        if bundle:
            all_facts = bundle.get("facts") or []
            picked = _pick_facts(all_facts, question, fact_limit)
            evidence["facts"] = picked
            detail_available = len(all_facts) > len(picked) or level == "brief"
            ver = (bundle.get("entity") or {}).get("game_version") or ""
            if intent == "facts":
                if picked:
                    top = picked[0]
                    draft = (
                        f"{label} 的 {top.get('key')} 为 {_fact_value(top)}"
                        f"{('（' + ver + '）') if ver else ''}。"
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
        commodity = primary or question
        rows = best_prices(conn, commodity, side=side, limit=price_limit)
        evidence["prices"] = _slim_prices(rows)
        if rows:
            top = rows[0]
            draft = (
                f"{top.get('commodity')} {side} 较优：{top.get('terminal')} "
                f"约 {top.get('price')} aUEC"
                f"{('（' + str(top.get('game_version')) + '）') if top.get('game_version') else ''}。"
            )
            detail_available = True
            follow_ups = ["看更多终端", "查买入价"] if side == "sell" else ["看更多终端", "查卖出价"]
        else:
            missing.append("prices")
            draft = f"本地没有 {commodity} 的 {side} 报价。"
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

    return {
        "intent": intent,
        "path": plan["path"],
        "confidence": plan["confidence"],
        "answer_draft": draft,
        "evidence": evidence,
        "detail_available": detail_available,
        "follow_ups": follow_ups,
        "browser": plan.get("browser") or {"needed": False},
        "plan": plan,
    }