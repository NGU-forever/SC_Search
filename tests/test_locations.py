from __future__ import annotations

import json
from pathlib import Path

from sc_search.ingest.adapters.scmdb import apply_scmdb_payload
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run
from sc_search.query.locations import list_locations

FIXTURE = Path(__file__).parent / "fixtures" / "scmdb_mini.json"


def test_list_locations_filters(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    payload = json.loads(FIXTURE.read_text(encoding="utf-8"))
    run_id = start_run(conn, "scmdb", payload["version"])
    apply_scmdb_payload(conn, payload, run_id=run_id, game_version=payload["version"])
    conn.commit()

    rows = list_locations(conn, query="RAB", system="Pyro", limit=10)
    assert rows
    assert any(r["canonical_name"] == "RAB-ALPHA" for r in rows)

    stations = list_locations(conn, location_type="Station", limit=10)
    assert any(r["canonical_name"] == "Checkmate" for r in stations)
    conn.close()
