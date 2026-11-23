"""Local Router/Worker debug CLI (no client LLM).

Examples::

    python -m sc_search.rw bypass "卡拉克货舱多大"
    python -m sc_search.rw run "黄金哪里卖最贵" --primary trade --slots '{"commodity":"黄金","side":"sell"}'
    python -m sc_search.rw run "…" --primary trade --slots '{}' --compact
    python -m sc_search.rw intents
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any


def _print(obj: Any) -> None:
    sys.stdout.write(json.dumps(obj, ensure_ascii=False, indent=2) + "\n")


def _compact_worker(plan: dict[str, Any], data: dict[str, Any]) -> dict[str, Any]:
    return {
        "primary": data.get("primary") or plan.get("primary"),
        "path": data.get("path") or plan.get("path"),
        "calls": [
            {
                "id": c.get("id"),
                "tool": c.get("tool"),
                "needs": c.get("needs") or [],
                "args": c.get("args") or {},
            }
            for c in (plan.get("calls") or [])
        ],
        "results": [
            {
                "id": r.get("id"),
                "tool": r.get("tool"),
                "ok": r.get("ok"),
                "error": r.get("error"),
                "augmented": bool(r.get("augmented")),
            }
            for r in (data.get("call_results") or [])
        ],
        "evidence_count": len(data.get("evidence") or []),
        "evidence_empty": data.get("evidence_empty"),
        "slot_retry_suggested": data.get("slot_retry_suggested"),
        "mismatch": data.get("mismatch"),
        "replan_suggested": data.get("replan_suggested"),
        "reasoning_hint": data.get("reasoning_hint") or "",
        "augmented_tools": [
            a.get("tool") for a in (data.get("augmented_calls") or [])
        ],
    }


def cmd_bypass(question: str, detail_level: str) -> int:
    from sc_search.mcp_server import server as mcp

    raw = json.loads(mcp.handlers["sc_answer"](question, detail_level))
    _print(raw)
    return 0 if raw.get("ok") else 1


def cmd_run(
    question: str,
    primary: str,
    slots: dict[str, Any],
    replan_count: int,
    *,
    compact: bool,
) -> int:
    from sc_search.mcp_server import server as mcp

    routed = json.loads(
        mcp.handlers["sc_route"](
            question,
            primary,
            json.dumps(slots, ensure_ascii=False),
            replan_count,
        )
    )
    if not routed.get("ok"):
        _print(routed)
        return 1
    plan = routed["data"]
    worked = json.loads(
        mcp.handlers["sc_worker_run"](json.dumps(plan, ensure_ascii=False))
    )
    if not worked.get("ok"):
        _print(worked)
        return 1
    data = worked.get("data") or {}
    if compact:
        _print(_compact_worker(plan, data))
    else:
        _print({"route": plan, "worker": data})
    if data.get("replan_suggested") or data.get("slot_retry_suggested"):
        return 2
    return 0


def cmd_intents() -> int:
    from sc_search.query.intent_tools import INTENT_TOOLS

    rows = []
    for name, pb in INTENT_TOOLS.items():
        rows.append(
            {
                "primary": name,
                "slots": pb.get("slots") or [],
                "tools": pb.get("tools") or [],
                "path": pb.get("path"),
                "notes": pb.get("notes") or "",
            }
        )
    _print(rows)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m sc_search.rw",
        description="Debug sc_route → sc_worker_run (or sc_answer bypass) against local DB",
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    p_bypass = sub.add_parser("bypass", help="One-shot sc_answer")
    p_bypass.add_argument("question")
    p_bypass.add_argument("--detail", default="brief")

    p_run = sub.add_parser("run", help="sc_route then sc_worker_run")
    p_run.add_argument("question")
    p_run.add_argument("--primary", required=True)
    p_run.add_argument(
        "--slots",
        default="{}",
        help='JSON object, e.g. \'{"commodity":"黄金","side":"sell"}\'',
    )
    p_run.add_argument("--replan-count", type=int, default=0)
    p_run.add_argument(
        "--compact",
        action="store_true",
        help="Print signals + call ok/fail only (no full evidence payloads)",
    )

    sub.add_parser("intents", help="List INTENT_TOOLS primary → slots/tools/path")

    args = parser.parse_args(argv)
    if args.cmd == "bypass":
        return cmd_bypass(args.question, args.detail)
    if args.cmd == "intents":
        return cmd_intents()
    try:
        slots = json.loads(args.slots)
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"invalid --slots JSON: {exc}\n")
        return 1
    if not isinstance(slots, dict):
        sys.stderr.write("--slots must be a JSON object\n")
        return 1
    return cmd_run(
        args.question,
        args.primary,
        slots,
        args.replan_count,
        compact=bool(args.compact),
    )


if __name__ == "__main__":
    raise SystemExit(main())
