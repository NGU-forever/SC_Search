from __future__ import annotations

from sc_search.query.answer.constants import _GUIDE_INTENTS

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
    if intent == "mission_list":
        return "high" if not missing else "low"
    if intent in _GUIDE_INTENTS:
        # Guides rely on LLM reasoning + citations; medium unless empty of everything.
        return "medium" if has_entity or not missing else "low"
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
