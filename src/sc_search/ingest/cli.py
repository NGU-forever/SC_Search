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


def _sync_result(source: str, rows: int, path, **extra) -> int:
    payload = {"ok": True, "source": source, "row_count": rows, "db": str(path), **extra}
    print(json.dumps(payload, ensure_ascii=False))
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
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("uex", rows, path)

        if args.source == "scmdb":
            from sc_search.ingest.adapters.scmdb import ScmdbError, sync_scmdb

            try:
                rows = sync_scmdb(conn, channel=args.channel, version=args.version)
                conn.commit()
            except ScmdbError as exc:
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("scmdb", rows, path, channel=args.channel)

        if args.source == "scw":
            from sc_search.ingest.adapters.scw_api import ScwError, sync_scw

            try:
                rows = sync_scw(conn)
                conn.commit()
            except ScwError as exc:
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("scw", rows, path)

        if args.source == "wiki":
            from sc_search.ingest.adapters.wiki import WikiError, sync_wiki

            try:
                rows = sync_wiki(conn)
                conn.commit()
            except WikiError as exc:
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("wiki", rows, path)

        if args.source == "aliases":
            from sc_search.ingest.adapters.aliases import AliasError, sync_aliases

            try:
                rows = sync_aliases(conn)
                conn.commit()
            except AliasError as exc:
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("aliases", rows, path)

        if args.source == "zh":
            from sc_search.ingest.adapters.localization import LocalizationError, sync_localization

            try:
                rows = sync_localization(conn)
                conn.commit()
            except LocalizationError as exc:
                conn.commit()
                print(json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False))
                return 1
            except Exception:
                conn.rollback()
                raise
            return _sync_result("zh", rows, path)

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
    p_sync.add_argument(
        "source",
        choices=["uex", "scmdb", "scw", "wiki", "aliases", "zh"],
        help="Source adapter name",
    )
    p_sync.add_argument(
        "--channel",
        default="live",
        choices=["live", "ptu", "latest"],
        help="SCMDB channel (ignored for other sources)",
    )
    p_sync.add_argument("--version", default=None, help="Optional exact SCMDB version substring")
    p_sync.set_defaults(func=cmd_sync)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return int(args.func(args))
