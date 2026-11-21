from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import (
    finish_run,
    insert_relation,
    replace_price,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)
from sc_search.query.answer import answer_question


def _seed(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    scmdb_run = start_run(conn, "scmdb", "4.10.3-live.13220821")
    finish_run(
        conn,
        scmdb_run,
        status="ok",
        row_count=1,
        game_version="4.10.3-live.13220821",
    )
    run = start_run(conn, "scw", "scw-live")
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
    upsert_fact(
        conn,
        entity_id=ship,
        key="manufacturer",
        value_text="RSI",
        source="scw",
        game_version="4.10.3",
        ingest_run_id=run,
    )
    for key, value_num, value_text in (
        ("length", 155.0, None),
        ("beam", 51.0, None),
        ("height", 25.0, None),
        ("mass", 3500.0, None),
        ("crew", None, "6"),
        ("hp", 12000.0, None),
    ):
        upsert_fact(
            conn,
            entity_id=ship,
            key=key,
            value_num=value_num,
            value_text=value_text,
            unit=None,
            source="scw",
            game_version="4.10.3",
            ingest_run_id=run,
        )
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
        game_version="3.24.2",
        ingest_run_id=run,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    rsi = upsert_entity(conn, kind="faction", canonical_name="RSI", game_version="4.10.3")
    insert_relation(
        conn,
        from_id=ship,
        to_id=rsi,
        rel_type="built_by",
        source="test",
        game_version="4.10.3",
    )
    unob = upsert_entity(conn, kind="commodity", canonical_name="Unobtanium", game_version="4.10.3")
    upsert_alias(conn, unob, "Unobtanium", "zh", "manual")
    conn.commit()
    conn.close()


@pytest.fixture
def conn(tmp_path: Path):
    db = tmp_path / "a.sqlite"
    _seed(db)
    c = connect(db)
    yield c
    c.close()


def test_facts_manufacturer_not_cargo(conn):
    r = answer_question(conn, "卡拉克厂家", detail_level="brief")
    assert r["intent"] == "facts"
    assert "RSI" in r["answer_draft"] or "manufacturer" in r["answer_draft"].lower()
    assert "456" not in r["answer_draft"]


def test_facts_cargo_brief(conn):
    r = answer_question(conn, "卡拉克货舱多大", detail_level="brief")
    assert r["intent"] == "facts"
    assert r["browser"]["needed"] is False
    assert "456" in r["answer_draft"] or "货舱" in r["answer_draft"]
    assert r["detail_available"] is True
    assert len(r["evidence"]["facts"]) <= 5
    assert r["plan"]["path"] == "mcp_only"


def test_facts_full_thicker(conn):
    brief = answer_question(conn, "卡拉克", detail_level="brief")
    full = answer_question(conn, "卡拉克", detail_level="full")
    assert len(brief["evidence"]["facts"]) == 5
    assert len(full["evidence"]["facts"]) > len(brief["evidence"]["facts"])
    assert full["evidence"]["relations"]
    assert full["evidence"]["relations"][0]["rel_type"] == "built_by"


def test_prices_terminal_query(conn):
    r = answer_question(conn, "Lorville 价格")
    assert r["intent"] == "prices"
    assert r["evidence"]["prices"]
    assert all(p.get("terminal") == "Lorville" for p in r["evidence"]["prices"])


def test_prices_sell(conn):
    r = answer_question(conn, "黄金在哪卖")
    assert r["intent"] == "prices"
    assert r["browser"]["needed"] is False
    assert "Lorville" in r["answer_draft"] or "250" in r["answer_draft"]
    assert r["evidence"]["prices"]
    assert r["live_game_version"] == "4.10.3"
    assert "Live 4.10.3" in r["answer_draft"]
    assert "3.24.2" in r["answer_draft"]  # UEX tag kept as 来源标注


def test_loadout_static_suggests_browser(conn):
    r = answer_question(conn, "卡拉克怎么配打炮")
    assert r["intent"] == "loadout_calc"
    assert "Erkul" in r["answer_draft"] or "配装" in r["answer_draft"]
    assert r["browser"]["needed"] is True
    assert r["browser"]["search_term"] == "Carrack"


def test_unknown_no_invent(conn):
    r = answer_question(conn, "完全不存在的飞船XYZ123货舱")
    assert "未覆盖" in r["answer_draft"] or "没有" in r["answer_draft"]
    assert r["detail_available"] is False


def test_cargo_draft_zh_and_high(conn):
    r = answer_question(conn, "卡拉克货舱多大")
    assert r["confidence"] == "high"
    assert "货舱" in r["answer_draft"]
    assert "cargo_capacity" not in r["answer_draft"] or "货舱" in r["answer_draft"]
    assert r["browser"]["needed"] is False
    assert r["evidence"]["facts"][0].get("key_zh") == "货舱"


def test_missing_field_soft_browser(conn):
    r = answer_question(conn, "卡拉克量子糖分是多少")
    assert r["confidence"] in {"low", "medium"}
    assert r["browser"]["needed"] is False
    assert any("浏览器核实" in f for f in r["follow_ups"])


def test_no_prices_low(conn):
    r = answer_question(conn, "Unobtanium在哪卖")
    assert r["intent"] == "prices"
    assert r["confidence"] == "low"
    assert r["browser"]["needed"] is False
    assert any("浏览器核实" in f for f in r["follow_ups"])


def test_detail_available_false_when_exhausted(conn):
    r = answer_question(conn, "卡拉克厂家", detail_level="full")
    assert r["detail_available"] is False