import json
from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import (
    finish_run,
    replace_price,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)
from tests.test_missions import _seed_missions
from sc_search.mcp_server import server as mcp_server


def _seed(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    run_id = start_run(conn, "uex", "4.10.3")
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    term = upsert_entity(conn, kind="terminal", canonical_name="Lorville", game_version="4.10.3")
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=term,
        side="sell",
        price=250.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run_id,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    finish_run(conn, run_id, status="ok", row_count=1, game_version="4.10.3")
    run_scw = start_run(conn, "scw", "4.10.3")
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
        ingest_run_id=run_scw,
    )
    conn.commit()
    conn.close()


def test_mcp_handlers(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "t.sqlite"
    _seed(db)
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    resolved = json.loads(mcp_server.handlers["sc_resolve_name"]("黄金"))
    assert resolved["ok"] is True
    assert resolved["data"][0]["canonical_name"] == "Gold"

    prices = json.loads(mcp_server.handlers["sc_best_prices"]("黄金", side="sell"))
    assert prices["ok"] is True
    assert prices["data"][0]["price"] == 250.0

    term = json.loads(mcp_server.handlers["sc_terminal_prices"]("Lorville"))
    assert term["ok"] is True

    fresh = json.loads(mcp_server.handlers["sc_data_freshness"]("uex"))
    assert fresh["ok"] is True
    assert fresh["data"][0]["status"] == "ok"

    plan = json.loads(mcp_server.handlers["sc_query_plan"]("卡拉克怎么配打炮"))
    assert plan["ok"] is True
    assert plan["data"]["intent"] == "loadout_calc"
    assert plan["data"]["browser"]["search_term"] == "Carrack"

    ans = json.loads(mcp_server.handlers["sc_answer"]("卡拉克货舱多大"))
    assert ans["ok"] is True
    assert ans["data"]["browser"]["needed"] is False
    assert ans["data"]["answer_draft"]


def test_mcp_sc_list_missions(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "missions.sqlite"
    conn = connect(db)
    init_db(conn)
    _seed_missions(conn)
    conn.close()
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    raw = json.loads(
        mcp_server.handlers["sc_list_missions"](
            faction="Red Wind Linehaul",
            rank_keyword="Junior",
            limit=5,
        )
    )
    assert raw["ok"] is True
    assert len(raw["data"]) >= 2
    assert all("Red Wind" in (r.get("contractor") or "") for r in raw["data"])


def test_mcp_list_missions_no_match_is_ok(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "missions.sqlite"
    conn = connect(db)
    init_db(conn)
    _seed_missions(conn)
    conn.close()
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    raw = json.loads(
        mcp_server.handlers["sc_list_missions"](
            faction="Nonexistent Faction XYZ",
            rank_keyword="zzzz-no-such",
            limit=5,
        )
    )
    assert raw["ok"] is True
    assert raw["data"] == []
    assert raw["meta"].get("why") == "no_match"
    assert "message" in raw["meta"]


def test_mcp_best_prices_no_match_is_ok(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "t.sqlite"
    _seed(db)
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    raw = json.loads(mcp_server.handlers["sc_best_prices"]("NoSuchCommodityXYZ", side="sell"))
    assert raw["ok"] is True
    assert raw["data"] == []
    assert raw["meta"].get("why") == "no_match"
