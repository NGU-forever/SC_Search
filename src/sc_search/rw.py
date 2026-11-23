"""Local Router/Worker debug CLI (no client LLM).

Examples::

    python -m sc_search.rw bypass "卡拉克货舱多大"
    python -m sc_search.rw run "黄金哪里卖最贵" --primary trade --slots '{"commodity":"黄金","side":"sell"}'
"""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any


def _print(obj: Any) -> None:
    sys.stdout.write(json.dumps(obj, ensure_ascii=False, indent=2) + "\n")


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
    out = {
        "route": plan,
        "worker": worked.get("data") if worked.get("ok") else worked,
    }
    _print(out)
    if not worked.get("ok"):
        return 1
    data = worked.get("data") or {}
    if data.get("replan_suggested") or data.get("slot_retry_suggested"):
        return 2
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

    args = parser.parse_args(argv)
    if args.cmd == "bypass":
        return cmd_bypass(args.question, args.detail)
    try:
        slots = json.loads(args.slots)
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"invalid --slots JSON: {exc}\n")
        return 1
    if not isinstance(slots, dict):
        sys.stderr.write("--slots must be a JSON object\n")
        return 1
    return cmd_run(args.question, args.primary, slots, args.replan_count)


if __name__ == "__main__":
    raise SystemExit(main())
