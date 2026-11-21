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
    skin = upsert_entity(
        conn, kind="item", canonical_name='CQ7 "Blacklist" Rifle', game_version="4.10.3"
    )
    upsert_alias(conn, skin, 'CQ7 "Blacklist" Rifle', "en", "manual")
    bp = upsert_entity(conn, kind="blueprint", canonical_name="CQ7 Rifle", game_version="4.10.3")
    upsert_alias(conn, bp, "CQ7 Rifle", "en", "manual")
    upsert_alias(conn, bp, "CQ7 步枪", "zh", "manual")
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
    assert p["mcp_steps"][-1]["args"]["side"] == "sell"


def test_prices_buy_side(conn):
    p = query_plan(conn, "黄金在哪买")
    assert p["intent"] == "prices"
    assert p["mcp_steps"][-1]["args"]["side"] == "buy"


def test_loadout_unresolved_no_chinese_search_term(conn):
    p = query_plan(conn, "怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["search_term"] == ""
    assert p["confidence"] in ("low", "medium")
    for step in p["mcp_steps"]:
        if step["tool"] == "sc_resolve_name":
            assert step["args"]["text"] != "怎么配打炮"


def test_mining_unresolved_no_chinese_search_term(conn):
    p = query_plan(conn, "采矿头怎么配")
    assert p["intent"] == "mining_calc"
    assert p["browser"]["search_term"] == ""
    assert p["confidence"] in ("low", "medium")
    for step in p["mcp_steps"]:
        if step["tool"] == "sc_resolve_name":
            assert step["args"]["text"] != "采矿头怎么配"


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


def test_shortcode_cq7_blueprint_context_prefers_blueprint(conn):
    q = "cq7的蓝图怎么获取以及我要制造的话，材料怎么得。"
    p = query_plan(conn, q)
    assert p["entities"], "expected token resolve to find CQ7 entities"
    kinds = {e["kind"] for e in p["entities"]}
    assert "blueprint" in kinds
    assert p["entities"][0]["kind"] == "blueprint"
    assert p["entities"][0]["canonical_name"] == "CQ7 Rifle"