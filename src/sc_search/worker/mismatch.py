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
    "mission_money": frozenset({"missions", "resolve", "facts", "answer"}),
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

# Content families that must be present for the primary — resolve alone must not
# suppress slot_retry when these are missing (e.g. missions for mission_*).
_STRONG: dict[str, frozenset[str]] = {
    "trade": frozenset({"prices"}),
    "trade_route": frozenset({"prices"}),
    "ship_stats": frozenset({"facts"}),
    "ship_buy": frozenset({"facts"}),
    "mission_rep": frozenset({"missions"}),
    "mission_money": frozenset({"missions"}),
    "blueprint": frozenset({"facts"}),
    "crafting": frozenset({"facts"}),
    "loadout": frozenset({"facts"}),
    "mining": frozenset({"facts"}),
    "bug_tip": frozenset({"docs"}),
    "client_help": frozenset({"docs"}),
    "howto": frozenset({"docs", "answer"}),
    "earn_meta": frozenset({"missions"}),
    "pledge_meta": frozenset({"docs"}),
    "event_guide": frozenset({"docs", "freshness"}),
    "lore_docs": frozenset({"docs"}),
    "patch_live": frozenset({"freshness"}),
    "other": frozenset(),
}

_TOOL_FAMILY = {
    "sc_best_prices": "prices",
    "sc_terminal_prices": "prices",
    "sc_entity_facts": "facts",
    "sc_resolve_name": "resolve",
    "sc_list_missions": "missions",
    "sc_locations": "locations",
    "sc_search_docs": "docs",
    "sc_data_freshness": "freshness",
    "sc_answer": "answer",
    "sc_query_plan": "plan",
}

# Families that do not satisfy "content" intents when they are the ONLY hits.
_WEAK_ALONE = frozenset({"docs", "freshness", "plan"})


def _data_useful(data: Any) -> bool:
    if data is None:
        return False
    if data == [] or data == {}:
        return False
    return True


def families_present(call_results: list[dict[str, Any]]) -> set[str]:
    found: set[str] = set()
    for row in call_results:
        if not row.get("ok"):
            continue
        data = row.get("data")
        if not _data_useful(data):
            continue
        fam = _TOOL_FAMILY.get(str(row.get("tool") or ""))
        if fam:
            found.add(fam)
    return found


def strong_family_missing(primary: str, call_results: list[dict[str, Any]]) -> bool:
    """True when primary expects a strong content family that is absent."""
    strong = _STRONG.get(primary)
    if strong is None:
        strong = _STRONG["other"]
    if not strong:
        return False
    present = families_present(call_results)
    return not bool(present & strong)


def collect_failed_calls(call_results: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Summarize error / no_match calls for client slot retry."""
    out: list[dict[str, Any]] = []
    for row in call_results:
        tool = row.get("tool")
        args = row.get("args") or {}
        meta = row.get("meta") or {}
        if not row.get("ok"):
            out.append({"tool": tool, "args": args, "reason": row.get("error") or "error"})
            continue
        if meta.get("why") == "no_match":
            out.append(
                {
                    "tool": tool,
                    "args": args,
                    "reason": meta.get("message") or "no_match",
                }
            )
    return out


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
    strong_missing: bool = False,
) -> bool:
    """Soft signal: empty evidence or missing strong family → retry slots/query."""
    if mismatch:
        return False
    if int(replan_count) >= int(limit):
        return False
    return bool(evidence_empty) or bool(strong_missing)
