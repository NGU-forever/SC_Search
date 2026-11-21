from pathlib import Path

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import finish_run, replace_price, start_run, upsert_alias, upsert_entity
from sc_search.query.freshness import data_freshness
from sc_search.query.prices import best_prices
from sc_search.query.resolve import resolve_name


def _seed(conn) -> None:
    run_id = start_run(conn, "uex", "4.10.3")
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    a18 = upsert_entity(conn, kind="terminal", canonical_name="Area18", game_version="4.10.3")
    lorville = upsert_entity(conn, kind="terminal", canonical_name="Lorville", game_version="4.10.3")
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=a18,
        side="sell",
        price=100.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run_id,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=lorville,
        side="sell",
        price=250.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run_id,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    finish_run(conn, run_id, status="ok", row_count=2, game_version="4.10.3")
    conn.commit()


def test_resolve_alias(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    _seed(conn)
    hits = resolve_name(conn, "黄金")
    assert hits
    assert hits[0]["canonical_name"] == "Gold"
    conn.close()


def test_best_prices_sell_orders_desc(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    _seed(conn)
    rows = best_prices(conn, "黄金", side="sell", limit=10)
    assert [r["terminal"] for r in rows] == ["Lorville", "Area18"]
    assert rows[0]["price"] == 250.0
    conn.close()


def test_data_freshness(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    _seed(conn)
    rows = data_freshness(conn, source="uex")
    assert rows
    assert rows[0]["status"] == "ok"
    conn.close()
