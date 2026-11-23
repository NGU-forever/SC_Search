import json
from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import (
    finish_run,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)
from sc_search.mcp_server import server as mcp_server
from sc_search.router.dag import expand_dag
from sc_search.worker.executor import run_plan


def _seed(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    run = start_run(conn, "scw", "4.10.3")
    ship = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, ship, "卡拉克", "zh", "manual")
    upsert_fact(
        conn,
        entity_id=ship,
        key="cargo_capacity",
        value_num=456.0,
        unit="SCU",
        source="scw",
        game_version="4.10.3",
        ingest_run_id=run,
    )
    finish_run(conn, run, status="ok", row_count=1, game_version="4.10.3")
    conn.commit()
    conn.close()


def test_worker_binds_resolve_and_facts(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    db = tmp_path / "w.sqlite"
    _seed(db)
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    plan = expand_dag("卡拉克货舱", "ship_stats", {"ship": "卡拉克"})
    # Drop auto facts from playbook to force augment path: keep only resolve
    plan.calls = [c for c in plan.calls if c["tool"] == "sc_resolve_name"]
    out = run_plan(plan.to_dict(), mcp_server.worker_handlers)
    assert out["ok"] is True
    tools = [r["tool"] for r in out["call_results"] if r.get("ok")]
    assert "sc_resolve_name" in tools
    assert "sc_entity_facts" in tools
    assert out["augmented_calls"]
    assert out["mismatch"] is False


def test_mcp_route_and_worker(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    db = tmp_path / "w2.sqlite"
    _seed(db)
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    routed = json.loads(
        mcp_server.handlers["sc_route"](
            "卡拉克货舱多大",
            "ship_stats",
            '{"ship":"卡拉克"}',
        )
    )
    assert routed["ok"] is True
    plan = routed["data"]
    assert any(c["tool"] == "sc_resolve_name" for c in plan["calls"])

    worked = json.loads(
        mcp_server.handlers["sc_worker_run"](json.dumps(plan, ensure_ascii=False))
    )
    assert worked["ok"] is True
    assert worked["data"]["ok"] is True
    assert worked["data"]["replan_suggested"] is False
