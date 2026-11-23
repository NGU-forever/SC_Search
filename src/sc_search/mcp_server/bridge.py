"""JSON bridge for Go MCP dual-track: invoke a named handler and print its JSON string.

Usage::

    python -m sc_search.mcp_server.bridge sc_resolve_name '{"text":"黄金"}'
"""

from __future__ import annotations

import json
import sys
from typing import Any

from sc_search.mcp_server import server as mcp_server


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) < 1:
        sys.stderr.write("usage: python -m sc_search.mcp_server.bridge TOOL [ARGS_JSON]\n")
        return 2
    tool = args[0]
    raw_args = args[1] if len(args) > 1 else "{}"
    try:
        payload: Any = json.loads(raw_args) if raw_args.strip() else {}
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"invalid ARGS_JSON: {exc}\n")
        return 2
    if not isinstance(payload, dict):
        sys.stderr.write("ARGS_JSON must be a JSON object\n")
        return 2
    handler = mcp_server.handlers.get(tool)
    if handler is None:
        sys.stdout.write(
            json.dumps(
                {"ok": False, "error": f"unknown tool: {tool}", "data": None, "meta": {}},
                ensure_ascii=False,
            )
            + "\n"
        )
        return 1
    try:
        # FastMCP-decorated callables still accept kwargs matching tool params.
        out = handler(**payload)
    except TypeError as exc:
        sys.stdout.write(
            json.dumps(
                {"ok": False, "error": f"bad args for {tool}: {exc}", "data": None, "meta": {}},
                ensure_ascii=False,
            )
            + "\n"
        )
        return 1
    except Exception as exc:  # noqa: BLE001 — bridge must always emit envelope
        sys.stdout.write(
            json.dumps(
                {"ok": False, "error": str(exc), "data": None, "meta": {}},
                ensure_ascii=False,
            )
            + "\n"
        )
        return 1
    if isinstance(out, str):
        sys.stdout.write(out if out.endswith("\n") else out + "\n")
    else:
        sys.stdout.write(json.dumps(out, ensure_ascii=False) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
