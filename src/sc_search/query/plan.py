from __future__ import annotations

import re
import sqlite3

from sc_search.query.guides import browser_guide, guides_for
from sc_search.query.resolve import _KIND_RANK, _rank_rows, resolve_name

ERKUL_CALCULATOR = "https://erkul.games/calculator"

_LOADOUT_RE = re.compile(
    r"(dps|loadout|erkul|配装|怎么配|如何配|打炮|火力|护盾续航)",
    re.I,
)
_MINING_RE = re.compile(
    r"(破矿|精炼模拟|regolith|激光头|采矿头|mining\s*laser)",
    re.I,
)
_PRICES_RE = re.compile(
    r"(价格|报价|最低价|最高价|最便宜|最贵|哪里买|哪里卖|在哪买|在哪卖|"
    r"哪[买卖出]|买点|卖点|买入|卖出|多少钱|\bbuy\b|\bsell\b|price)",
    re.I,
)
_MISSION_LIST_RE = re.compile(
    r"(有哪些任务|哪些任务|任务列表|什么任务|刷什么任务|任务有哪些)",
    re.I,
)
_CLIENT_RE = re.compile(
    r"(报错|进不去|打不开|锁服|汉化|虚拟内存|鼠标失灵|19000|32212|"
    r"删\s*user|角色重置|reset\b|ptu怎么进|eptu|启动器)",
    re.I,
)
_HOWTO_RE = re.compile(
    r"(怎么走|如何走|叫船|机库|床上?登出|按键|键位|跃迁怎么|量子怎么|"
    r"怎么起飞|怎么降落|对接|找路|路线|操作教程|新手教程|"
    r"怎么上(我的)?飞?船|复活点|出生点)",
    re.I,
)
_EARN_META_RE = re.compile(
    r"(怎么赚钱|如何赚钱|怎么刷钱|开荒|萌新(该|要)?干什么|新手(该|要)?干什么|"
    r"第一桶金|赚钱方法|推荐玩法|怎么玩)",
    re.I,
)
_PLEDGE_RE = re.compile(
    r"(ccu|warbond|\bwb\b|融船|信用点|store\s*credit|叠叠乐|退款|灰市|"
    r"star\s*hangar|资格包|新手包|邀请码|lti)",
    re.I,
)
_EVENT_RE = re.compile(
    r"(890\s*登舰|奥里森之围|海盗潮|iae|free\s*fly|免费试飞|活动怎么|"
    r"北极星兑换|巴努商店)",
    re.I,
)
_TRADE_ROUTE_RE = re.compile(
    r"(跑商航线|贸易航线|推荐航线|整条线|最优路线)",
    re.I,
)
_SHIP_SHOP_RE = re.compile(
    r"(哪里租船|租船|购点|游戏里哪里买船|船店|new\s*deal)",
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
_BUY_RE = re.compile(r"(买|买入|哪里买|在哪买|\bbuy\b)", re.I)
_SELL_RE = re.compile(r"(卖|卖出|哪里卖|在哪卖|哪出|\bsell\b)", re.I)
_OBTAIN_CTX_RE = re.compile(
    r"(哪里买|在哪买|能买到|买得到|多少钱|获取|怎么得|如何得|掉落|奖励|"
    r"\bbuy\b|\bobtain\b|买)",
    re.I,
)
# Short codes like CQ7, JS-400 (must include a digit to avoid English stopwords).
_SHORTCODE_RE = re.compile(
    r"(?<![A-Za-z0-9])([A-Za-z]{1,8}\d+[A-Za-z0-9\-]*|[A-Za-z]+\-\d+[A-Za-z0-9]*)(?![A-Za-z0-9])"
)
# Letter-only names (Parallax, Coda) — used in blueprint context; kept only if resolve hits.
_NAME_TOKEN_RE = re.compile(r"(?<![A-Za-z0-9])([A-Za-z]{4,16})(?![A-Za-z0-9])")
_BLUEPRINT_CTX_RE = re.compile(
    r"(蓝图|制造|制作|配方|材料|获取|crafting|blueprint)",
    re.I,
)
_NAME_TOKEN_STOP = frozenset(
    {
        "blueprint",
        "blueprints",
        "crafting",
        "craft",
        "materials",
        "material",
        "where",
        "what",
        "when",
        "with",
        "from",
        "this",
        "that",
        "have",
        "need",
        "make",
        "made",
        "obtain",
        "price",
        "prices",
        "buy",
        "sell",
        "loadout",
        "rifle",
        "pistol",
        "energy",
        "assault",
        "weapon",
        "weapons",
    }
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


# Single-char CJK commodity shorts used in CN community (铝哪里买).
# Grammar/noise glyphs (大/红/多) are excluded — localization often aliases them.
_CJK_SINGLE_ALLOW = frozenset("铝铁钨金铜钛钢锡铅锌钴镍镁锂铍银")


def _alias_len_ok(alias: str) -> bool:
    a = alias.strip()
    if not a:
        return False
    # Keep ASCII shortcodes gated (avoid "at"/"to").
    if a.isascii():
        return len(a) >= 3
    if len(a) == 1:
        return a in _CJK_SINGLE_ALLOW
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
          AND length(trim(a.alias)) >= 1
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
    if not items:
        return []
    # Prefer longer alias hits (卡拉克 > noise), then kind rank.
    ranked = _rank_rows(items)
    ranked.sort(
        key=lambda m: (
            -len(str(m.get("matched_alias") or "")),
            _kind_tiebreak(m),
            str(m.get("canonical_name") or ""),
        )
    )
    return ranked


def _kind_tiebreak(m: dict) -> int:
    return int(_KIND_RANK.get(str(m.get("kind")), 99))


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


def _extract_name_tokens(question: str) -> list[str]:
    found: list[str] = []
    seen: set[str] = set()
    for m in _NAME_TOKEN_RE.finditer(question or ""):
        tok = m.group(1)
        key = tok.lower()
        if key in _NAME_TOKEN_STOP or key in seen:
            continue
        seen.add(key)
        found.append(tok)
    return found


def _question_tokens(question: str) -> list[str]:
    return _extract_shortcodes(question) + _extract_name_tokens(question)


def _resolve_blueprints_for_token(conn: sqlite3.Connection, token: str) -> list[dict]:
    """LIKE match blueprints only — avoids item skins crowding out resolve_name top-N."""
    tok = (token or "").strip()
    if len(tok) < 2:
        return []
    like = f"%{tok}%"
    rows = conn.execute(
        """
        SELECT DISTINCT e.id, e.kind, e.canonical_name, e.slug, e.uuid, e.game_version,
               COALESCE(a.alias, e.canonical_name) AS matched_alias,
               COALESCE(a.lang, 'en') AS matched_lang
        FROM entities e
        LEFT JOIN aliases a ON a.entity_id = e.id
        WHERE e.kind = 'blueprint'
          AND (
            e.canonical_name LIKE ? COLLATE NOCASE
            OR a.alias LIKE ? COLLATE NOCASE
          )
        ORDER BY length(e.canonical_name) ASC
        LIMIT 25
        """,
        (like, like),
    ).fetchall()
    return [dict(r) for r in rows]


def _blueprint_sort_key(entity: dict, tokens: list[str]) -> tuple:
    name = str(entity.get("canonical_name") or "")
    lower = name.casefold()
    skin_penalty = 1 if '"' in name or "“" in name or "”" in name else 0
    token_rank = 99
    for i, tok in enumerate(tokens):
        t = tok.casefold()
        if lower == t or lower.startswith(t + " ") or lower.startswith(t + "-"):
            # Prefer exact/base forms over longer skin titles sharing the prefix.
            token_rank = min(token_rank, i)
    return (skin_penalty, token_rank, len(name), lower)


def _wants_blueprint_prefer(question: str) -> bool:
    q = question or ""
    return bool(_BLUEPRINT_CTX_RE.search(q) or _OBTAIN_CTX_RE.search(q))


def _prefer_blueprint_primary(entities: list[dict], question: str) -> list[dict]:
    if not entities or not _wants_blueprint_prefer(question):
        return entities
    bps = [e for e in entities if e.get("kind") == "blueprint"]
    if not bps:
        # Obtain/buy context: still lift blueprints found only via shortcode merge later;
        # when items crowd the list, try to surface any blueprint already present.
        return entities
    tokens = _question_tokens(question)
    bps_sorted = sorted(bps, key=lambda e: _blueprint_sort_key(e, tokens))
    rest = [e for e in entities if e.get("kind") != "blueprint"]
    return bps_sorted + rest


def _commodity_or_terminal_primary(entities: list[dict]) -> bool:
    if not entities:
        return True
    return entities[0].get("kind") in {"commodity", "terminal"}


def _faction_primary(entities: list[dict]) -> dict | None:
    for e in entities:
        if e.get("kind") == "faction":
            return e
    return None


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
        prefer_bp = _wants_blueprint_prefer(q)
        tokens = _extract_shortcodes(q)
        if prefer_bp:
            tokens = tokens + _extract_name_tokens(q)
        for tok in tokens:
            hits = resolve_name(conn, tok)
            if prefer_bp:
                # Merge blueprint-only hits so skins don't crowd out base BPs in top-N.
                seen_hit = {int(m["id"]) for m in hits}
                for m in _resolve_blueprints_for_token(conn, tok):
                    if int(m["id"]) not in seen_hit:
                        hits.append(m)
                        seen_hit.add(int(m["id"]))
            if not hits:
                continue
            for m in hits:
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


def _dedupe_resources(rows: list[dict]) -> list[dict]:
    seen: set[str] = set()
    out: list[dict] = []
    for r in rows:
        url = str(r.get("url") or "")
        if not url or url in seen:
            continue
        seen.add(url)
        out.append(r)
    return out


def _browser_erkul(search_term: str) -> dict:
    resources = _dedupe_resources(
        [
            {"title": "Erkul Calculator", "url": ERKUL_CALCULATOR, "kind": "tool"},
            *guides_for("loadout", limit=3),
        ]
    )
    return {
        "needed": True,
        "site": "erkul",
        "start_url": ERKUL_CALCULATOR,
        "search_term": search_term,
        "resources": resources,
        "max_actions": 8,
        "extract": ["selected ship", "DPS summary", "shield / power notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
        "wait_confirm": True,
    }


def _browser_mining(search_term: str) -> dict:
    resources = _dedupe_resources(guides_for("mining", limit=5))
    start = resources[0]["url"] if resources else ""
    return {
        "needed": True,
        "site": "mining",
        "start_url": start,
        "search_term": search_term,
        "resources": resources,
        "max_actions": 8,
        "extract": ["laser / module setup", "instability / resistance notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
        "wait_confirm": True,
    }


def _guide_plan(
    *,
    intent: str,
    category: str,
    entities: list[dict],
    resolve_text: str,
    reason: str,
    mcp_steps: list[dict] | None = None,
) -> dict:
    steps = list(mcp_steps or [])
    if resolve_text and not any(s.get("tool") == "sc_resolve_name" for s in steps):
        steps.insert(0, {"tool": "sc_resolve_name", "args": {"text": resolve_text}})
    return {
        "intent": intent,
        "path": "mcp_then_browser",
        "confidence": "medium",
        "entities": entities,
        "mcp_steps": steps,
        "browser": browser_guide(category),
        "stop_if": "Agent 可用本地证据+推理成文；仅当用户确认或要逐步核对时再开浏览器",
        "reason": reason,
        "guide_category": category,
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

    if _MISSION_LIST_RE.search(q):
        faction = _faction_primary(entities)
        if faction:
            fname = str(faction["canonical_name"])
            # Faction first so answer labels use the contractor, not a color item.
            ents = [faction] + [e for e in entities if e.get("id") != faction.get("id")]
            return {
                "intent": "mission_list",
                "path": "mcp_only",
                "confidence": "high",
                "entities": ents,
                "mcp_steps": [
                    *(
                        [{"tool": "sc_resolve_name", "args": {"text": resolve_text}}]
                        if resolve_text
                        else []
                    ),
                    {
                        "tool": "sc_list_missions",
                        "args": {"faction": fname, "limit": 8},
                    },
                ],
                "browser": _browser_off(),
                "stop_if": "任务列表已够则不要开浏览器",
                "reason": "势力任务列表走本地 SCMDB",
            }

    if _PRICES_RE.search(q) and _commodity_or_terminal_primary(entities):
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

    # Guide-style intents: local evidence + curated links; LLM reasons; browser on confirm.
    if _CLIENT_RE.search(q):
        return _guide_plan(
            intent="client_help",
            category="client",
            entities=entities,
            resolve_text=resolve_text,
            reason="客户端/报错/汉化需教程外链；本地不编造排障步骤",
        )
    if _PLEDGE_RE.search(q):
        return _guide_plan(
            intent="pledge_meta",
            category="pledge",
            entities=entities,
            resolve_text=resolve_text,
            reason="氪金/CCU/退款名词需官方与社区指南；库无账单数据",
        )
    if _EVENT_RE.search(q):
        return _guide_plan(
            intent="event_guide",
            category="event",
            entities=entities,
            resolve_text=resolve_text,
            reason="活动本/试飞时效强，给外链供 Agent 推理引用",
        )
    if _SHIP_SHOP_RE.search(q):
        steps = []
        if primary:
            steps.append({"tool": "sc_entity_facts", "args": {"name_or_id": primary}})
        return _guide_plan(
            intent="ship_shop",
            category="ship_shop",
            entities=entities,
            resolve_text=resolve_text,
            reason="船店/租借购点以 Everark 等外链为准；本地可附静态参数",
            mcp_steps=steps,
        )
    if _TRADE_ROUTE_RE.search(q):
        return _guide_plan(
            intent="trade_route",
            category="trade_route",
            entities=entities,
            resolve_text=resolve_text,
            reason="整条跑商航线需规划站；可辅以本地单价",
        )
    if _EARN_META_RE.search(q) and not _faction_primary(entities):
        return _guide_plan(
            intent="earn_meta",
            category="earn_meta",
            entities=entities,
            resolve_text=resolve_text,
            reason="赚钱开荒需推理组织玩法；附任务/Wiki 外链，不编造时薪",
            mcp_steps=[{"tool": "sc_list_missions", "args": {"limit": 5}}],
        )
    if _HOWTO_RE.search(q):
        return _guide_plan(
            intent="howto",
            category="howto",
            entities=entities,
            resolve_text=resolve_text,
            reason="操作/找路教程类；本地证据+外链供 LLM 成文",
        )

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