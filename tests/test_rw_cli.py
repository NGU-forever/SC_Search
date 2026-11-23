import json
from pathlib import Path

import pytest

from sc_search import rw
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import (
    finish_run,
    replace_price,
    start_run,
    upsert_alias,
    upsert_entity,
)


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
    conn.commit()
    conn.close()


def test_rw_run_trade(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]) -> None:
    db = tmp_path / "rw.sqlite"
    _seed(db)
    monkeypatch.setenv("SC_SEARCH_DB", str(db))
    code = rw.main(
        [
            "run",
            "黄金哪里卖最贵",
            "--primary",
            "trade",
            "--slots",
            '{"commodity":"黄金","side":"sell"}',
        ]
    )
    assert code == 0
    out = json.loads(capsys.readouterr().out)
    assert out["route"]["primary"] == "trade"
    assert out["worker"]["evidence_empty"] is False
    assert out["worker"]["mismatch"] is False


def test_rw_bad_slots() -> None:
    assert rw.main(["run", "q", "--primary", "trade", "--slots", "not-json"]) == 1
