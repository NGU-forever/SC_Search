from __future__ import annotations

from sc_search.query.answer.constants import (
    _CARGO_KEYS,
    _FACT_KEY_ZH,
    _RELATION_LIMIT,
    _ZH_KEY_HINTS,
)

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

def _empty_evidence() -> dict:
    return {"entities": [], "facts": [], "relations": [], "prices": [], "docs": [], "missing": []}

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
