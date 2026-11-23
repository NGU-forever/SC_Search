"""Execute a route DAG in parallel waves; augment; mismatch / empty-slot signals."""

from __future__ import annotations

import json
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any, Callable

from sc_search.worker.augment import maybe_augment
from sc_search.worker.bind import bind_args
from sc_search.worker.mismatch import (
    check_mismatch,
    evidence_is_empty,
    replan_suggested,
    slot_retry_suggested,
)

Handler = Callable[..., str]

_MAX_WORKERS = 4


def _invoke(handlers: dict[str, Handler], tool: str, args: dict[str, Any]) -> dict[str, Any]:
    fn = handlers.get(tool)
    if fn is None:
        return {"ok": False, "error": f"unknown tool {tool!r}", "data": None, "meta": {}}
    try:
        raw = fn(**args)
        parsed = json.loads(raw)
        if not isinstance(parsed, dict):
            return {"ok": False, "error": "non-object tool response", "data": None, "meta": {}}
        return parsed
    except TypeError as exc:
        return {"ok": False, "error": f"bad args for {tool}: {exc}", "data": None, "meta": {}}
    except Exception as exc:
        return {"ok": False, "error": str(exc), "data": None, "meta": {}}


def _waves(
    calls: list[dict[str, Any]],
    handlers: dict[str, Handler],
) -> tuple[list[dict[str, Any]], dict[str, dict[str, Any]]]:
    pending = {c["id"]: dict(c) for c in calls}
    results: dict[str, dict[str, Any]] = {}
    ordered: list[dict[str, Any]] = []

    while pending:
        ready = [
            cid
            for cid, c in pending.items()
            if all(n in results for n in (c.get("needs") or []))
        ]
        if not ready:
            for cid, c in list(pending.items()):
                row = {
                    "id": cid,
                    "tool": c.get("tool"),
                    "ok": False,
                    "error": "unsatisfied needs",
                    "data": None,
                    "meta": {},
                    "args": c.get("args") or {},
                }
                results[cid] = row
                ordered.append(row)
            break

        def _run(cid: str) -> tuple[str, dict[str, Any]]:
            c = pending[cid]
            tool = str(c.get("tool") or "")
            bound = bind_args(dict(c.get("args") or {}), results)
            parsed = _invoke(handlers, tool, bound)
            row = {
                "id": cid,
                "tool": tool,
                "args": bound,
                "ok": bool(parsed.get("ok")),
                "error": parsed.get("error"),
                "data": parsed.get("data"),
                "meta": parsed.get("meta") or {},
            }
            if c.get("augmented"):
                row["augmented"] = True
            return cid, row

        with ThreadPoolExecutor(max_workers=min(_MAX_WORKERS, len(ready))) as pool:
            futs = [pool.submit(_run, cid) for cid in ready]
            for fut in as_completed(futs):
                cid, row = fut.result()
                results[cid] = row
                ordered.append(row)
                pending.pop(cid, None)

    return ordered, results


def run_plan(
    plan: dict[str, Any],
    handlers: dict[str, Handler],
) -> dict[str, Any]:
    primary = str(plan.get("primary") or "other")
    replan_count = int(plan.get("replan_count") or 0)
    question = str(plan.get("question") or "")
    calls = [dict(c) for c in (plan.get("calls") or []) if isinstance(c, dict)]

    call_results, by_id = _waves(calls, handlers)

    augmented_calls: list[dict[str, Any]] = []
    # Wave 1: resolve→facts; Wave 2: empty-intent docs/answer (sees prior results).
    for _ in range(2):
        extra = maybe_augment(
            calls + augmented_calls,
            by_id,
            primary=primary,
            question=question,
        )
        if not extra:
            break
        aug_results, aug_by_id = _waves(extra, handlers)
        call_results.extend(aug_results)
        by_id.update(aug_by_id)
        augmented_calls.extend(extra)

    evidence = [
        {
            "id": r["id"],
            "tool": r.get("tool"),
            "ok": r.get("ok"),
            "data": r.get("data"),
            "meta": r.get("meta") or {},
        }
        for r in call_results
        if r.get("ok") and r.get("data") is not None
    ]

    mismatch = check_mismatch(primary, call_results)
    empty = evidence_is_empty(evidence)
    suggest_replan = replan_suggested(mismatch, replan_count, limit=1)
    suggest_slots = slot_retry_suggested(
        evidence_empty=empty,
        mismatch=mismatch,
        replan_count=replan_count,
        limit=1,
    )

    if suggest_replan:
        hint = "证据类型与意图明显不符，请改 primary/slots 后重跑（仅一次）"
    elif suggest_slots:
        hint = "证据为空，请改 slots/检索词（英文或标准译名）后重跑；勿把开荒/赚钱等口语当 rank_keyword"
    else:
        hint = plan.get("notes") or ""

    return {
        "ok": True,
        "primary": primary,
        "slots": plan.get("slots") or {},
        "question": question,
        "path": plan.get("path") or "mcp_only",
        "notes": plan.get("notes") or "",
        "replan_count": replan_count,
        "evidence": evidence,
        "call_results": call_results,
        "augmented_calls": augmented_calls,
        "mismatch": mismatch,
        "replan_suggested": suggest_replan,
        "evidence_empty": empty,
        "slot_retry_suggested": suggest_slots,
        "reasoning_hint": hint,
    }
