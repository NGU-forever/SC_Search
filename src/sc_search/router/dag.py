"""Expand primary+slots into a parallelizable call DAG (local, deterministic)."""

from __future__ import annotations

from typing import Any

from sc_search.query.intent_tools import INTENT_TOOLS, playbook_for
from sc_search.router.schema import RoutePlan, validate_route
from sc_search.worker.slots import docs_query, mission_rank_keyword

# Worker path skips one-shot sc_answer (kept as client bypass).
_SKIP_IN_WORKER = frozenset({"sc_answer"})

# When resolve is in the same playbook, these wait on it.
_NEEDS_RESOLVE = frozenset(
    {
        "sc_entity_facts",
        "sc_list_missions",
        "sc_best_prices",
        "sc_terminal_prices",
    }
)

_RESOLVE_SLOT_KEYS = (
    "faction",
    "ship",
    "commodity",
    "item",
    "blueprint",
    "laser",
    "terminal",
)


def _resolve_text(slots: dict[str, Any], question: str) -> str:
    for key in _RESOLVE_SLOT_KEYS:
        val = slots.get(key)
        if isinstance(val, str) and val.strip():
            return val.strip()
    return (question or "").strip()


def _docs_query(slots: dict[str, Any], question: str) -> str:
    for key in ("topic", "query", "event", "activity", "goal"):
        val = slots.get(key)
        if isinstance(val, str) and val.strip():
            return docs_query(val, question)
    return docs_query(None, question)


def _args_for(
    tool: str,
    *,
    slots: dict[str, Any],
    question: str,
    resolve_id: str | None,
) -> dict[str, Any]:
    if tool == "sc_resolve_name":
        return {"text": _resolve_text(slots, question)}
    if tool == "sc_entity_facts":
        if resolve_id:
            return {"name_or_id": f"${resolve_id}.canonical"}
        return {"name_or_id": _resolve_text(slots, question)}
    if tool == "sc_list_missions":
        args: dict[str, Any] = {}
        if resolve_id and slots.get("faction"):
            args["faction"] = f"${resolve_id}.canonical"
        elif slots.get("faction"):
            args["faction"] = slots["faction"]
        rank = mission_rank_keyword(
            slots.get("rank") if isinstance(slots.get("rank"), str) else None,
            slots.get("rank_keyword")
            if isinstance(slots.get("rank_keyword"), str)
            else None,
            slots.get("activity") if isinstance(slots.get("activity"), str) else None,
        )
        if rank:
            args["rank_keyword"] = rank
        if slots.get("system"):
            args["system"] = slots["system"]
        return args
    if tool == "sc_best_prices":
        commodity = slots.get("commodity") or _resolve_text(slots, question)
        if resolve_id and slots.get("commodity"):
            commodity = f"${resolve_id}.canonical"
        elif resolve_id and not slots.get("commodity"):
            commodity = f"${resolve_id}.canonical"
        args = {
            "commodity": commodity,
            "side": slots.get("side") or "sell",
        }
        if slots.get("system"):
            args["system"] = slots["system"]
        return args
    if tool == "sc_terminal_prices":
        terminal = slots.get("terminal") or _resolve_text(slots, question)
        if resolve_id and slots.get("terminal"):
            terminal = f"${resolve_id}.canonical"
        args = {"terminal": terminal}
        if slots.get("side"):
            args["side"] = slots["side"]
        return args
    if tool == "sc_search_docs":
        return {"query": _docs_query(slots, question)}
    if tool == "sc_data_freshness":
        return {}
    if tool == "sc_query_plan":
        args = {"question": question}
        hint = _resolve_text(slots, question)
        if hint and hint != question:
            args["hint_entity"] = hint
        return args
    return {}


def expand_dag(
    question: str,
    primary: str | None,
    slots: dict[str, Any] | None = None,
    *,
    replan_count: int = 0,
) -> RoutePlan:
    key, cleaned = validate_route(primary, slots)
    pb = playbook_for(key) or INTENT_TOOLS["other"]
    tools = [t for t in (pb.get("tools") or []) if t not in _SKIP_IN_WORKER]
    # patch_live / docs-only may leave empty after skip — keep freshness/docs
    if not tools:
        tools = [t for t in (pb.get("tools") or []) if t in _SKIP_IN_WORKER]
        if not tools:
            tools = ["sc_resolve_name"]

    has_resolve = "sc_resolve_name" in tools
    resolve_id = "r1" if has_resolve else None
    # Skip resolve when no useful text
    if has_resolve and not _resolve_text(cleaned, question):
        tools = [t for t in tools if t != "sc_resolve_name"]
        has_resolve = False
        resolve_id = None

    # trade without terminal: drop terminal_prices
    if key == "trade" and not cleaned.get("terminal"):
        tools = [t for t in tools if t != "sc_terminal_prices"]
    # mission_money: resolve only useful if faction present
    if key == "mission_money" and not cleaned.get("faction"):
        tools = [t for t in tools if t != "sc_resolve_name"]
        has_resolve = "sc_resolve_name" in tools
        resolve_id = "r1" if has_resolve else None

    calls: list[dict[str, Any]] = []
    seq = 0

    for tool in tools:
        seq += 1
        cid = f"c{seq}"
        if tool == "sc_resolve_name":
            cid = "r1"
        needs: list[str] = []
        if tool in _NEEDS_RESOLVE and resolve_id and tool != "sc_resolve_name":
            needs = [resolve_id]
        args = _args_for(
            tool,
            slots=cleaned,
            question=question,
            resolve_id=resolve_id if tool in _NEEDS_RESOLVE else None,
        )
        # list_missions without faction: no resolve dependency
        if tool == "sc_list_missions" and not cleaned.get("faction"):
            needs = []
            args = _args_for(
                tool, slots=cleaned, question=question, resolve_id=None
            )
        calls.append({"id": cid, "tool": tool, "args": args, "needs": needs})

    return RoutePlan(
        primary=key,
        slots=cleaned,
        question=question,
        calls=calls,
        path=str(pb.get("path") or "mcp_only"),
        notes=str(pb.get("notes") or ""),
        replan_count=max(0, int(replan_count)),
        plan_intent=pb.get("plan_intent"),
    )
