from __future__ import annotations

import json
import sqlite3
from typing import Any

from mcp.server.fastmcp import FastMCP

from sc_search.ingest.db import connect
from sc_search.paths import get_db_path
from sc_search.query.answer import answer_question
from sc_search.query.entities import entity_facts
from sc_search.query.freshness import data_freshness, live_game_version
from sc_search.query.prices import best_prices, terminal_prices
from sc_search.query.plan import query_plan
from sc_search.query.resolve import resolve_name
from sc_search.query.missions import list_missions
from sc_search.query.search import search_docs

# Pin to MCP Python SDK 1.x FastMCP: host tool discovery is more reliable
# than the 2.x MCPServer API (which often surfaces as "live tool discovery" failure).
mcp = FastMCP("sc-search")


def _meta_from_rows(rows: list[dict]) -> dict[str, Any]:
    if not rows:
        return {}
    first = rows[0]
    return {
        "game_version": first.get("game_version"),
        "synced_at": first.get("reported_at") or first.get("finished_at"),
    }


def _ok(data: Any, meta: dict[str, Any] | None = None) -> str:
    return json.dumps({"ok": True, "data": data, "meta": meta or {}}, ensure_ascii=False)


def _err(message: str) -> str:
    return json.dumps({"ok": False, "error": message, "data": None, "meta": {}}, ensure_ascii=False)


def open_db() -> sqlite3.Connection:
    path = get_db_path()
    if not path.exists():
        raise FileNotFoundError(f"database missing: {path}; run: python -m sc_search.ingest init")
    return connect(path)


@mcp.tool()
def sc_resolve_name(text: str) -> str:
    """Resolve Star Citizen Chinese/English/slang names to entities in the local DB."""
    try:
        conn = open_db()
        try:
            data = resolve_name(conn, text)
            return _ok(data)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_best_prices(
    commodity: str,
    side: str = "sell",
    system: str | None = None,
    limit: int = 10,
) -> str:
    """Best buy/sell prices for a commodity across terminals (local UEX snapshot)."""
    try:
        conn = open_db()
        try:
            data = best_prices(conn, commodity, side=side, system=system, limit=limit)
            if not data:
                return _err(f"no prices for commodity={commodity!r} side={side!r}")
            return _ok(data, _meta_from_rows(data))
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_terminal_prices(terminal: str, side: str | None = None) -> str:
    """List commodity prices at a terminal from the local UEX snapshot."""
    try:
        conn = open_db()
        try:
            data = terminal_prices(conn, terminal, side=side)
            if not data:
                return _err(f"no prices for terminal={terminal!r}")
            return _ok(data, _meta_from_rows(data))
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_data_freshness(source: str | None = None) -> str:
    """Show last ingest_runs freshness (game_version and sync time) per source."""
    try:
        conn = open_db()
        try:
            data = data_freshness(conn, source=source)
            return _ok(data, _meta_from_rows(data))
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_entity_facts(name_or_id: str) -> str:
    """Facts and relations for a mission, blueprint, ship, item, or faction from local DB."""
    try:
        conn = open_db()
        try:
            data = entity_facts(conn, name_or_id)
            if data is None:
                return _err(f"entity not found: {name_or_id!r}")
            meta = {"game_version": data["entity"].get("game_version")}
            return _ok(data, meta)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_query_plan(question: str, hint_entity: str | None = None) -> str:
    """Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape)."""
    try:
        conn = open_db()
        try:
            data = query_plan(conn, question, hint_entity=hint_entity)
            return _ok(data)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_answer(
    question: str,
    detail_level: str = "brief",
    hint_entity: str | None = None,
) -> str:
    """One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape)."""
    try:
        conn = open_db()
        try:
            data = answer_question(
                conn, question, detail_level=detail_level, hint_entity=hint_entity
            )
            meta = {
                "game_version": data.get("live_game_version") or live_game_version(conn),
            }
            return _ok(data, meta)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_list_missions(
    faction: str | None = None,
    rank_keyword: str | None = None,
    system: str | None = None,
    limit: int = 10,
) -> str:
    """List SCMDB missions filtered by contractor faction, rank keyword in title/name, or system."""
    try:
        conn = open_db()
        try:
            data = list_missions(
                conn,
                faction=faction,
                rank_keyword=rank_keyword,
                system=system,
                limit=limit,
            )
            if not data:
                return _err(
                    "no missions matched filters"
                    + (f" faction={faction!r}" if faction else "")
                    + (f" rank_keyword={rank_keyword!r}" if rank_keyword else "")
                    + (f" system={system!r}" if system else "")
                )
            return _ok(data, _meta_from_rows(data))
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_search_docs(query: str, limit: int = 5) -> str:
    """Full-text search Star Citizen wiki extracts stored locally (FTS5)."""
    try:
        conn = open_db()
        try:
            data = search_docs(conn, query, limit=limit)
            if not data:
                return _err(f"no documents matched query={query!r}")
            return _ok(data, _meta_from_rows(data))
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))


def _parse_slots(slots_json: str | None) -> dict[str, Any]:
    if not slots_json or not str(slots_json).strip():
        return {}
    raw = json.loads(slots_json)
    if not isinstance(raw, dict):
        raise ValueError("slots_json must be a JSON object")
    return raw


# Data-plane handlers (Worker invokes these; no router/worker meta-tools).
worker_handlers = {
    "sc_answer": sc_answer,
    "sc_query_plan": sc_query_plan,
    "sc_resolve_name": sc_resolve_name,
    "sc_best_prices": sc_best_prices,
    "sc_terminal_prices": sc_terminal_prices,
    "sc_data_freshness": sc_data_freshness,
    "sc_entity_facts": sc_entity_facts,
    "sc_list_missions": sc_list_missions,
    "sc_search_docs": sc_search_docs,
}


@mcp.tool()
def sc_route(
    question: str,
    primary: str,
    slots_json: str = "{}",
    replan_count: int = 0,
) -> str:
    """Expand primary+slots into a parallel DAG plan (no DB fetch). Client LLM picks primary."""
    try:
        from sc_search.router.dag import expand_dag

        slots = _parse_slots(slots_json)
        plan = expand_dag(
            question,
            primary,
            slots,
            replan_count=replan_count,
        )
        return _ok(plan.to_dict())
    except Exception as exc:
        return _err(str(exc))


@mcp.tool()
def sc_worker_run(plan_json: str) -> str:
    """Execute a sc_route DAG in parallel waves; may augment; signals replan on evidence mismatch."""
    try:
        from sc_search.worker.executor import run_plan

        plan = json.loads(plan_json)
        if not isinstance(plan, dict):
            return _err("plan_json must be a JSON object")
        data = run_plan(plan, worker_handlers)
        return _ok(data)
    except Exception as exc:
        return _err(str(exc))


# Expose undecorated handlers for unit tests
handlers = {
    **worker_handlers,
    "sc_route": sc_route,
    "sc_worker_run": sc_worker_run,
}
