import json
from pathlib import Path

import pytest

from sc_search.ingest.adapters.scmdb import apply_scmdb_payload
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run
from sc_search.mcp_server import server as mcp_server
from sc_search.query.entities import entity_facts

FIXTURE = Path(__file__).parent / "fixtures" / "scmdb_mini.json"


def _seed(conn) -> None:
    payload = json.loads(FIXTURE.read_text(encoding="utf-8"))
    run_id = start_run(conn, "scmdb", payload["version"])
    apply_scmdb_payload(conn, payload, run_id=run_id, game_version=payload["version"])
    conn.commit()


def test_entity_facts_mission(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    _seed(conn)
    data = entity_facts(conn, "Foxwell_Test_Mission")
    assert data is not None
    assert data["entity"]["kind"] == "mission"
    by_title = entity_facts(conn, "Orange Level Contract: Test Patrol")
    assert by_title is not None
    keys = {f["key"] for f in data["facts"]}
    assert "reward_uec" in keys
    assert any(r["rel_type"] == "mission_reward_blueprint_pool" for r in data["relations"])
    conn.close()


def test_mcp_sc_entity_facts(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "t.sqlite"
    conn = connect(db)
    init_db(conn)
    _seed(conn)
    conn.close()
    monkeypatch.setenv("SC_SEARCH_DB", str(db))

    raw = mcp_server.handlers["sc_entity_facts"]("Test Helmet Blueprint")
    body = json.loads(raw)
    assert body["ok"] is True
    assert body["data"]["entity"]["kind"] == "blueprint"
