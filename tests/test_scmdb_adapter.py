import json
from pathlib import Path

from sc_search.ingest.adapters.scmdb import ScmdbClient, apply_scmdb_payload
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run


FIXTURE = Path(__file__).parent / "fixtures" / "scmdb_mini.json"


def test_scmdb_apply_fixture(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    payload = json.loads(FIXTURE.read_text(encoding="utf-8"))
    run_id = start_run(conn, "scmdb", payload["version"])
    rows = apply_scmdb_payload(conn, payload, run_id=run_id, game_version=payload["version"])
    conn.commit()
    assert rows > 0

    missions = conn.execute(
        "SELECT COUNT(*) AS c FROM entities WHERE kind = 'mission'"
    ).fetchone()["c"]
    assert missions == 2

    bps = conn.execute(
        "SELECT COUNT(*) AS c FROM entities WHERE kind = 'blueprint'"
    ).fetchone()["c"]
    assert bps == 2

    rel = conn.execute(
        """
        SELECT COUNT(*) AS c FROM relations
        WHERE rel_type = 'mission_reward_blueprint_pool'
        """
    ).fetchone()["c"]
    assert rel == 1

    uec = conn.execute(
        """
        SELECT value_num FROM facts
        WHERE key = 'reward_uec'
        """
    ).fetchone()["value_num"]
    assert uec == 50000.0

    cost = conn.execute(
        "SELECT value_num FROM facts WHERE key = 'cost_uec'"
    ).fetchone()["value_num"]
    assert cost == 33000.0
    cooldown = conn.execute(
        "SELECT value_num FROM facts WHERE key = 'cooldown'"
    ).fetchone()["value_num"]
    assert cooldown == 45.0
    lifetime = conn.execute(
        "SELECT value_num FROM facts WHERE key = 'lifetime'"
    ).fetchone()["value_num"]
    assert lifetime == 4.0
    site = conn.execute(
        "SELECT value_text FROM facts WHERE key = 'site_pool'"
    ).fetchone()["value_text"]
    assert "RAB-ALPHA" in site
    assert "Checkmate" in site

    locs = conn.execute(
        "SELECT COUNT(*) AS c FROM entities WHERE kind = 'location'"
    ).fetchone()["c"]
    assert locs == 3
    rel_loc = conn.execute(
        "SELECT COUNT(*) AS c FROM relations WHERE rel_type = 'mission_location'"
    ).fetchone()["c"]
    assert rel_loc == 3
    conn.close()


def test_scmdb_client_sync_with_payload(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    payload = json.loads(FIXTURE.read_text(encoding="utf-8"))
    client = ScmdbClient(raw_dir=tmp_path / "raw")
    rows = client.sync(conn, payload=payload)
    conn.commit()
    assert rows > 0
    run = conn.execute(
        "SELECT status, game_version FROM ingest_runs ORDER BY id DESC LIMIT 1"
    ).fetchone()
    assert run["status"] == "ok"
    assert run["game_version"] == "4.10.3-live.test"
    client.close()
    conn.close()
