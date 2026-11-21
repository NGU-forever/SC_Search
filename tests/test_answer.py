from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import replace_price, start_run, upsert_alias, upsert_entity, upsert_fact
from sc_search.query.answer import answer_question


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
    upsert_fact(
        conn,
        entity_id=ship,
        key="manufacturer",
        value_text="RSI",
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
        game_version="4.10.3",
        ingest_run_id=run,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    conn.commit()
    conn.close()


@pytest.fixture
def conn(tmp_path: Path):
    db = tmp_path / "a.sqlite"
    _seed(db)
    c = connect(db)
    yield c
    c.close()


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
    assert len(full["evidence"]["facts"]) >= len(brief["evidence"]["facts"])


def test_prices_sell(conn):
    r = answer_question(conn, "黄金在哪卖")
    assert r["intent"] == "prices"
    assert r["browser"]["needed"] is False
    assert "Lorville" in r["answer_draft"] or "250" in r["answer_draft"]
    assert r["evidence"]["prices"]


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