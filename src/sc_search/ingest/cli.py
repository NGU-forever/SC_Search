from __future__ import annotations

import argparse
import json

from sc_search.ingest.db import connect, init_db
from sc_search.paths import get_db_path


def cmd_init(_: argparse.Namespace) -> int:
    path = get_db_path()
    conn = connect(path)
    init_db(conn)
    conn.close()
    print(f"initialized {path}")
    return 0


def cmd_status(_: argparse.Namespace) -> int:
    path = get_db_path()
    if not path.exists():
        print(json.dumps({"ok": False, "error": f"db missing: {path}"}, ensure_ascii=False))
        return 1
    conn = connect(path)
    rows = conn.execute(
        """
        SELECT id, source, game_version, started_at, finished_at, status, row_count, notes
        FROM ingest_runs
        ORDER BY id DESC
        LIMIT 20
        """
    ).fetchall()
    conn.close()
    data = [dict(r) for r in rows]
    print(json.dumps({"ok": True, "db": str(path), "runs": data}, ensure_ascii=False, indent=2))
    return 0


def cmd_sync(args: argparse.Namespace) -> int:
    path = get_db_path()
    conn = connect(path)
    init_db(conn)
    try:
        if args.source == "uex":
            from sc_search.ingest.adapters.uex import UexError, sync_uex

            try:
                rows = sync_uex(conn)
                conn.commit()
            except UexError as exc:
                conn.commit()  # keep failed ingest_runs row
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            print(json.dumps({"ok": True, "source": "uex", "row_count": rows, "db": str(path)}, ensure_ascii=False))
            return 0
        print(json.dumps({"ok": False, "error": f"unknown source: {args.source}"}, ensure_ascii=False))
        return 1
    finally:
        conn.close()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="sc_search.ingest")
    sub = parser.add_subparsers(dest="command", required=True)

    p_init = sub.add_parser("init", help="Create SQLite schema")
    p_init.set_defaults(func=cmd_init)

    p_status = sub.add_parser("status", help="Show recent ingest_runs")
    p_status.set_defaults(func=cmd_status)

    p_sync = sub.add_parser("sync", help="Sync a data source into SQLite")
    p_sync.add_argument("source", choices=["uex"], help="Source adapter name")
    p_sync.set_defaults(func=cmd_sync)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return int(args.func(args))
