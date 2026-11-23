"""Allow ``python -m sc_search`` to show a short pointer to subcommands."""

from __future__ import annotations

import sys


def main() -> int:
    sys.stderr.write(
        "sc_search modules:\n"
        "  python -m sc_search.mcp_server   # MCP stdio server\n"
        "  python -m sc_search.ingest …     # sync / init\n"
        "  python -m sc_search.rw bypass|run …  # Router/Worker debug CLI\n"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
