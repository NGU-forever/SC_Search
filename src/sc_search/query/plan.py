from __future__ import annotations

import re
import sqlite3

from sc_search.query.resolve import resolve_name

ERKUL_CALCULATOR = "https://erkul.games/calculator"
REGOLITH = "https://regolith.rocks"

_LOADOUT_RE = re.compile(
    r"(dps|loadout|erkul|配装|怎么配|如何配|打炮|火力|护盾续航)",
    re.I,
)
_MINING_RE = re.compile(
    r"(破矿|精炼模拟|regolith|激光头|采矿头|mining\s*laser)",
    re.I,
)
_PRICES_RE = re.compile(
    r"(价格|报价|哪[买卖出]|买点|卖点|\bbuy\b|\bsell\b|price)",
    re.I,
)
_LIVE_RE = re.compile(
    r"(patch\s*notes?|spectrum|补丁说明|官方论坛)",
    re.I,
)
_DOCS_RE = re.compile(
    r"(lore|设定|背景|wiki\s*摘要|是什么来头)",
    re.I,
)


def _entity_brief(m: dict) -> dict:
    brief = {
        "id": m["id"],
        "kind": m["kind"],
        "canonical_name": m["canonical_name"],
    }
    lang = m.get("matched_lang")
    alias = m.get("matched_alias")
    if lang == "zh" and alias:
        brief["zh"] = alias
    return brief


def _resolve_entities(conn: sqlite3.Connection, texts: list[str]) -> list[dict]:
    seen: set[int] = set()
    out: list[dict] = []
    for t in texts:
        t = (t or "").strip()
        if not t:
            continue
        for m in resolve_name(conn, t):
            eid = int(m["id"])
            if eid in seen:
                continue
            seen.add(eid)
            out.append(_entity_brief(m))
    return out


def _browser_off() -> dict:
    return {"needed": False}


def _browser_erkul(search_term: str) -> dict:
    return {
        "needed": True,
        "site": "erkul",
        "start_url": ERKUL_CALCULATOR,
        "search_term": search_term,
        "max_actions": 8,
        "extract": ["selected ship", "DPS summary", "shield / power notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
    }


def _browser_mining(search_term: str) -> dict:
    return {
        "needed": True,
        "site": "regolith",
        "start_url": REGOLITH,
        "search_term": search_term,
        "max_actions": 8,
        "extract": ["laser / module setup", "instability / resistance notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
    }


def query_plan(
    conn: sqlite3.Connection,
    question: str,
    hint_entity: str | None = None,
) -> dict:
    q = (question or "").strip()
    hint = (hint_entity or "").strip() or None

    resolve_texts = [t for t in [hint, q] if t]
    entities = _resolve_entities(conn, resolve_texts)
    primary = entities[0]["canonical_name"] if entities else (hint or "")

    if _MINING_RE.search(q):
        return {
            "intent": "mining_calc",
            "path": "mcp_then_browser",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": (
                [{"tool": "sc_resolve_name", "args": {"text": hint or q}}] if q else []
            ),
            "browser": _browser_mining(primary or q),
            "stop_if": "只要矿船静态参数时不开采矿站",
            "reason": "采矿配装/模拟需交互站",
        }

    if _LOADOUT_RE.search(q):
        steps = []
        if q:
            steps.append({"tool": "sc_resolve_name", "args": {"text": hint or q}})
        if primary:
            steps.append({"tool": "sc_entity_facts", "args": {"name_or_id": primary}})
        return {
            "intent": "loadout_calc",
            "path": "mcp_then_browser",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": steps,
            "browser": _browser_erkul(primary or q),
            "stop_if": "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
            "reason": "配装/DPS 需 Erkul；库无实时计算",
        }

    if _PRICES_RE.search(q):
        commodity = primary or q
        return {
            "intent": "prices",
            "path": "mcp_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_resolve_name", "args": {"text": hint or q}},
                {"tool": "sc_best_prices", "args": {"commodity": commodity, "side": "sell"}},
            ],
            "browser": _browser_off(),
            "stop_if": "有报价则不要开浏览器",
            "reason": "物价走本地 UEX 快照",
        }

    if _LIVE_RE.search(q):
        return {
            "intent": "live_web",
            "path": "browser_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [],
            "browser": {
                "needed": True,
                "site": "rsi",
                "start_url": "https://robertsspaceindustries.com",
                "search_term": q,
                "max_actions": 8,
                "extract": ["patch version", "relevant notes"],
                "do_not": ["open /auth", "/settings"],
            },
            "stop_if": "",
            "reason": "Patch/Spectrum 需官网或论坛",
        }

    if _DOCS_RE.search(q):
        return {
            "intent": "docs",
            "path": "mcp_only",
            "confidence": "medium",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_search_docs", "args": {"query": q, "limit": 5}},
            ],
            "browser": _browser_off(),
            "stop_if": "库有摘要则不要开浏览器",
            "reason": "本地 Wiki FTS",
        }

    if entities:
        name = entities[0]["canonical_name"]
        return {
            "intent": "facts",
            "path": "mcp_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_resolve_name", "args": {"text": hint or q}},
                {"tool": "sc_entity_facts", "args": {"name_or_id": name}},
            ],
            "browser": _browser_off(),
            "stop_if": "facts 已够答则不要开浏览器",
            "reason": "静态实体事实走本地库",
        }

    return {
        "intent": "facts",
        "path": "mcp_only",
        "confidence": "low",
        "entities": [],
        "mcp_steps": [{"tool": "sc_resolve_name", "args": {"text": q}}],
        "browser": _browser_off(),
        "stop_if": "未解析到实体前不要盲目开浏览器",
        "reason": "无法判断意图；先 resolve",
    }