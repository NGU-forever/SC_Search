from pathlib import Path

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import replace_price, start_run, upsert_alias, upsert_entity


def test_upsert_entity_is_idempotent(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    a = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    b = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    assert a == b
    count = conn.execute("SELECT COUNT(*) AS c FROM entities").fetchone()["c"]
    assert count == 1
    conn.close()


def test_replace_price_keeps_single_row(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    run_id = start_run(conn, "uex", "4.10.3")
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    term = upsert_entity(conn, kind="terminal", canonical_name="Area18", game_version="4.10.3")
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=term,
        side="sell",
        price=100.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run_id,
    )
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=term,
        side="sell",
        price=250.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run_id,
    )
    rows = conn.execute("SELECT price FROM prices").fetchall()
    assert len(rows) == 1
    assert rows[0]["price"] == 250.0
    conn.close()


def test_upsert_alias(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    count = conn.execute("SELECT COUNT(*) AS c FROM aliases").fetchone()["c"]
    assert count == 1
    conn.close()
