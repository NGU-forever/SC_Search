import json
from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import finish_run, replace_price, start_run, upsert_alias, upsert_entity
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
    ship = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, ship, "卡拉克", "zh", "manual")
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
