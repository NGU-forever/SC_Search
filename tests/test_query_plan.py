from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import upsert_alias, upsert_entity
from sc_search.query.plan import query_plan


def _seed_ship(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    eid = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, eid, "卡拉克", "zh", "manual")
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    conn.commit()
    conn.close()


@pytest.fixture
def conn(tmp_path: Path):
    db = tmp_path / "p.sqlite"
    _seed_ship(db)
    c = connect(db)
    yield c
    c.close()


def test_facts_cargo_mcp_only(conn):
    p = query_plan(conn, "卡拉克货舱多大")
    assert p["intent"] == "facts"
    assert p["path"] == "mcp_only"
    assert p["browser"]["needed"] is False
    assert p["entities"][0]["canonical_name"] == "Carrack"


def test_prices_mcp_only(conn):
    p = query_plan(conn, "黄金在哪卖")
    assert p["intent"] == "prices"
    assert p["path"] == "mcp_only"
    assert p["browser"]["needed"] is False


def test_loadout_zh(conn):
    p = query_plan(conn, "卡拉克怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["needed"] is True
    assert p["browser"]["start_url"] == "https://erkul.games/calculator"
    assert p["browser"]["search_term"] == "Carrack"
    assert p["browser"]["max_actions"] == 8


def test_loadout_en(conn):
    p = query_plan(conn, "Carrack best DPS loadout")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["search_term"] == "Carrack"


def test_mining_calc(conn):
    p = query_plan(conn, "鼹鼠怎么配激光头")
    assert p["intent"] == "mining_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["start_url"] == "https://regolith.rocks"