from __future__ import annotations

import re
import sqlite3

from sc_search.query.resolve import _rank_rows, resolve_name

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
_BUY_RE = re.compile(r"(买|\bbuy\b)", re.I)
_SELL_RE = re.compile(r"(卖|哪出|\bsell\b)", re.I)
# Short codes like CQ7, JS-400 (must include a digit to avoid English stopwords).
_SHORTCODE_RE = re.compile(
    r"(?<![A-Za-z0-9])([A-Za-z]{1,8}\d+[A-Za-z0-9\-]*|[A-Za-z]+\-\d+[A-Za-z0-9]*)(?![A-Za-z0-9])"
)
_BLUEPRINT_CTX_RE = re.compile(
    r"(蓝图|制造|制作|配方|材料|获取|crafting|blueprint)",
    re.I,
)


def _mcp_resolve_text(hint: str | None, entities: list[dict]) -> str:
    h = (hint or "").strip()
    if h:
        return h
    if entities:
        e = entities[0]
        if e.get("zh"):
            return str(e["zh"])
        return str(e["canonical_name"])
    return ""


def _browser_search_term(entities: list[dict]) -> str:
    if entities:
        return str(entities[0]["canonical_name"])
    return ""


def _price_side(question: str) -> str:
    q = question or ""
    if _BUY_RE.search(q):
        return "buy"
    if _SELL_RE.search(q):
        return "sell"
    return "sell"


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


def _alias_len_ok(alias: str) -> bool:
    a = alias.strip()
    if len(a) < 2:
        return False
    if a.isascii() and len(a) < 3:
        return False
    return True


def _entities_embedded_in_question(conn: sqlite3.Connection, question: str) -> list[dict]:
    q = (question or "").strip()
    if not q:
        return []

    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               a.alias AS matched_alias, a.lang AS matched_lang
        FROM aliases a
        JOIN entities e ON e.id = a.entity_id
        WHERE (? LIKE '%' || a.alias || '%') COLLATE NOCASE
          AND length(trim(a.alias)) >= 2
        ORDER BY length(a.alias) DESC
        LIMIT 25
        """,
        (q,),
    ).fetchall()
    items = [dict(r) for r in rows if _alias_len_ok(str(r["matched_alias"] or ""))]
    if not items:
        rows = conn.execute(
            """
            SELECT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
                   e.canonical_name AS matched_alias, 'en' AS matched_lang
            FROM entities e
            WHERE (? LIKE '%' || e.canonical_name || '%') COLLATE NOCASE
              AND length(trim(e.canonical_name)) >= 2
            ORDER BY length(e.canonical_name) DESC
            LIMIT 25
            """,
            (q,),
        ).fetchall()
        items = [dict(r) for r in rows if _alias_len_ok(str(r["matched_alias"] or ""))]
    return _rank_rows(items) if items else []


def _extract_shortcodes(question: str) -> list[str]:
    found: list[str] = []
    seen: set[str] = set()
    for m in _SHORTCODE_RE.finditer(question or ""):
        tok = m.group(1)
        key = tok.lower()
        if key in seen:
            continue
        seen.add(key)
        found.append(tok)
    return found


def _prefer_blueprint_primary(entities: list[dict], question: str) -> list[dict]:
    if not entities or not _BLUEPRINT_CTX_RE.search(question or ""):
        return entities
    bps = [e for e in entities if e.get("kind") == "blueprint"]
    if not bps:
        return entities
    rest = [e for e in entities if e.get("kind") != "blueprint"]
    return bps + rest


def _resolve_entities(
    conn: sqlite3.Connection,
    hint: str | None,
    question: str,
) -> list[dict]:
    seen: set[int] = set()
    out: list[dict] = []
    for t in [hint, question]:
        t = (t or "").strip()
        if not t:
            continue
        for m in resolve_name(conn, t):
            eid = int(m["id"])
            if eid in seen:
                continue
            seen.add(eid)
            out.append(_entity_brief(m))
    q = (question or "").strip()
    if q:
        for tok in _extract_shortcodes(q):
            for m in resolve_name(conn, tok):
                eid = int(m["id"])
                if eid in seen:
                    continue
                seen.add(eid)
                out.append(_entity_brief(m))
        for m in _entities_embedded_in_question(conn, q):
            eid = int(m["id"])
            if eid in seen:
                continue
            seen.add(eid)
            out.append(_entity_brief(m))
    return _prefer_blueprint_primary(out, q)


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

    entities = _resolve_entities(conn, hint, q)
    resolve_text = _mcp_resolve_text(hint, entities)
    search_term = _browser_search_term(entities)
    primary = entities[0]["canonical_name"] if entities else ""

    # Overlapping 怎么配 + mining terms must prefer mining_calc (e.g. 鼹鼠怎么配激光头).
    if _MINING_RE.search(q):
        conf = "high" if search_term else "medium"
        steps: list[dict] = []
        if resolve_text:
            steps.append({"tool": "sc_resolve_name", "args": {"text": resolve_text}})
        return {
            "intent": "mining_calc",
            "path": "mcp_then_browser",
            "confidence": conf,
            "entities": entities,
            "mcp_steps": steps,
            "browser": _browser_mining(search_term),
            "stop_if": "只要矿船静态参数时不开采矿站",
            "reason": "采矿配装/模拟需交互站",
        }

    if _LOADOUT_RE.search(q):
        conf = "high" if search_term else "medium"
        steps = []
        if resolve_text:
            steps.append({"tool": "sc_resolve_name", "args": {"text": resolve_text}})
        if primary:
            steps.append({"tool": "sc_entity_facts", "args": {"name_or_id": primary}})
        return {
            "intent": "loadout_calc",
            "path": "mcp_then_browser",
            "confidence": conf,
            "entities": entities,
            "mcp_steps": steps,
            "browser": _browser_erkul(search_term),
            "stop_if": "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
            "reason": "配装/DPS 需 Erkul；库无实时计算",
        }

    if _PRICES_RE.search(q):
        commodity = primary or resolve_text
        side = _price_side(q)
        return {
            "intent": "prices",
            "path": "mcp_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [
                *(
                    [{"tool": "sc_resolve_name", "args": {"text": resolve_text}}]
                    if resolve_text
                    else []
                ),
                {"tool": "sc_best_prices", "args": {"commodity": commodity, "side": side}},
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
                *(
                    [{"tool": "sc_resolve_name", "args": {"text": resolve_text}}]
                    if resolve_text
                    else []
                ),
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
        "mcp_steps": (
            [{"tool": "sc_resolve_name", "args": {"text": resolve_text}}] if resolve_text else []
        ),
        "browser": _browser_off(),
        "stop_if": "未解析到实体前不要盲目开浏览器",
        "reason": "无法判断意图；先 resolve",
    }