"""Worker may append whitelist tool calls (resolve→facts; empty meta intents)."""

from __future__ import annotations

from typing import Any

from sc_search.query.intent_tools import MCP_TOOLS
from sc_search.worker.bind import canonical_from_resolve

_AUGMENT_FACTS = frozenset({"sc_entity_facts"})
_EMPTY_AUGMENT_PRIMARIES = frozenset({"earn_meta", "howto", "bug_tip"})
_EMPTY_AUGMENT_TOOLS = ("sc_search_docs", "sc_answer")


def _has_ok_data(results: dict[str, dict[str, Any]], tool: str) -> bool:
    for row in results.values():
        if row.get("tool") != tool:
            continue
        if not row.get("ok"):
            continue
        data = row.get("data")
        if data is None or data == [] or data == {}:
            continue
        return True
    return False


def maybe_augment(
    calls: list[dict[str, Any]],
    results: dict[str, dict[str, Any]],
    *,
    primary: str | None = None,
    question: str | None = None,
) -> list[dict[str, Any]]:
    """Append facts after resolve; on empty meta intents, docs/answer with question."""
    existing_tools = {c.get("tool") for c in calls}
    existing_ids = {c.get("id") for c in calls}
    for row in results.values():
        existing_ids.add(row.get("id"))
    extra: list[dict[str, Any]] = []

    if "sc_entity_facts" not in existing_tools:
        resolve_ids = [
            c["id"]
            for c in calls
            if c.get("tool") == "sc_resolve_name" and c.get("id") in results
        ]
        for rid in resolve_ids:
            row = results[rid]
            if not row.get("ok"):
                continue
            canon = canonical_from_resolve(row.get("data"))
            if not canon:
                continue
            cid = f"aug_facts_{rid}"
            if cid in existing_ids:
                continue
            tool = "sc_entity_facts"
            if tool not in MCP_TOOLS or tool not in _AUGMENT_FACTS:
                continue
            extra.append(
                {
                    "id": cid,
                    "tool": tool,
                    "args": {"name_or_id": canon},
                    "needs": [],
                    "augmented": True,
                }
            )
            existing_ids.add(cid)

    key = (primary or "").strip()
    q = (question or "").strip()
    if key in _EMPTY_AUGMENT_PRIMARIES and q:
        strong_ok = any(
            _has_ok_data(results, t)
            for t in ("sc_list_missions", "sc_search_docs", "sc_answer", "sc_entity_facts")
        )
        if not strong_ok:
            for tool in _EMPTY_AUGMENT_TOOLS:
                if tool not in MCP_TOOLS:
                    continue
                # Skip if plan already had this tool with same question and it failed —
                # still retry once with explicit question arg.
                cid = f"aug_empty_{tool}"
                if cid in existing_ids:
                    continue
                if tool == "sc_search_docs":
                    args: dict[str, Any] = {"query": q}
                else:
                    args = {"question": q}
                extra.append(
                    {
                        "id": cid,
                        "tool": tool,
                        "args": args,
                        "needs": [],
                        "augmented": True,
                    }
                )
                existing_ids.add(cid)

    return extra
