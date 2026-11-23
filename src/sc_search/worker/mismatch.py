"""Rule 3 only: evidence family clearly disagrees with primary."""

from __future__ import annotations

from typing import Any

# Expected evidence families per primary (any match ⇒ ok).
_EXPECTED: dict[str, frozenset[str]] = {
    "trade": frozenset({"prices"}),
    "trade_route": frozenset({"prices"}),
    "ship_stats": frozenset({"facts", "resolve"}),
    "ship_buy": frozenset({"facts", "resolve", "docs"}),
    "mission_rep": frozenset({"missions", "resolve", "facts"}),
    "mission_money": frozenset({"missions", "resolve", "answer"}),
    "blueprint": frozenset({"facts", "resolve", "answer"}),
    "crafting": frozenset({"facts", "resolve", "answer"}),
    "loadout": frozenset({"facts", "resolve", "answer"}),
    "mining": frozenset({"facts", "resolve", "answer"}),
    "bug_tip": frozenset({"docs", "answer"}),
    "client_help": frozenset({"docs"}),
    "howto": frozenset({"docs", "answer"}),
    "earn_meta": frozenset({"missions", "docs", "answer"}),
    "pledge_meta": frozenset({"docs"}),
    "event_guide": frozenset({"docs", "freshness"}),
    "lore_docs": frozenset({"docs", "answer"}),
    "patch_live": frozenset({"freshness"}),
    "other": frozenset({"answer", "resolve", "plan", "facts", "docs", "missions", "prices"}),
}

_TOOL_FAMILY = {
    "sc_best_prices": "prices",
    "sc_terminal_prices": "prices",
    "sc_entity_facts": "facts",
    "sc_resolve_name": "resolve",
    "sc_list_missions": "missions",
    "sc_search_docs": "docs",
    "sc_data_freshness": "freshness",
    "sc_answer": "answer",
    "sc_query_plan": "plan",
}

# Families that do not satisfy "content" intents when they are the ONLY hits.
_WEAK_ALONE = frozenset({"docs", "freshness", "plan"})


def families_present(call_results: list[dict[str, Any]]) -> set[str]:
    found: set[str] = set()
    for row in call_results:
        if not row.get("ok"):
            continue
        data = row.get("data")
        if data is None or data == [] or data == {}:
            continue
        fam = _TOOL_FAMILY.get(str(row.get("tool") or ""))
        if fam:
            found.add(fam)
    return found


def check_mismatch(primary: str, call_results: list[dict[str, Any]]) -> bool:
    """True when evidence clearly mismatches primary (rule 3)."""
    expected = _EXPECTED.get(primary) or _EXPECTED["other"]
    present = families_present(call_results)
    if not present:
        # Empty evidence is not rule 3 (user chose only mismatch-of-type).
        return False
    if present & expected:
        # If only weak families hit but primary expects stronger ones exclusively
        # beyond weak — e.g. trade got only docs.
        strong = expected - _WEAK_ALONE
        if strong and not (present & strong) and (present <= _WEAK_ALONE):
            return True
        return False
    return True


def replan_suggested(mismatch: bool, replan_count: int, limit: int = 1) -> bool:
    return bool(mismatch) and int(replan_count) < int(limit)


def evidence_is_empty(evidence: list[dict[str, Any]] | None) -> bool:
    return not bool(evidence)


def slot_retry_suggested(
    *,
    evidence_empty: bool,
    mismatch: bool,
    replan_count: int,
    limit: int = 1,
) -> bool:
    """Soft signal: empty evidence → retry slots/query (not primary type mismatch)."""
    if mismatch:
        return False
    if not evidence_empty:
        return False
    return int(replan_count) < int(limit)
