import json
from pathlib import Path

import pytest

from sc_search.ingest.adapters.aliases import apply_seed_aliases, sync_aliases
from sc_search.ingest.adapters.scw_api import apply_scw_vehicles, sync_scw
from sc_search.ingest.adapters.wiki import sync_wiki, wiki_title_candidates
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run, upsert_entity
from sc_search.mcp_server import server as mcp_server
from sc_search.query.resolve import resolve_name
from sc_search.query.search import search_docs


SAMPLE_VEHICLE = {
    "uuid": "uuid-carrack",
    "name": "Carrack",
    "game_name": "Anvil Carrack",
    "slug": "anvl-carrack",
    "cargo_capacity": 456,
    "manufacturer": {"name": "Anvil Aerospace", "code": "ANVL"},
    "size": {"en_EN": "Large", "zh_CN": "大"},
    "type": {"en_EN": "Exploration", "zh_CN": "探索"},
    "foci": [{"en_EN": "Exploration", "zh_CN": "探索"}],
    "description": {
        "en_EN": "The Anvil Carrack is a multi-crew explorer.",
        "zh_CN": "Anvil Carrack 是多成员探索船。",
    },
    "mass": 4200000,
}


def test_wiki_title_candidates() -> None:
    assert "Carrack" in wiki_title_candidates("Anvil Carrack")


def test_scw_and_aliases_and_fts(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    rows = sync_scw(conn, vehicles=[SAMPLE_VEHICLE])
    assert rows == 1
    sync_aliases(
        conn,
        entries=[{"alias": "大白鲨", "target": "Carrack", "lang": "slang", "kind": "ship"}],
        probe_citizenwiki=False,
    )
    hits = resolve_name(conn, "大白鲨")
    assert hits
    assert "Carrack" in hits[0]["canonical_name"]

    sync_wiki(
        conn,
        documents=[
            {
                "title": "Carrack",
                "body": "The Anvil Carrack is a multi-crew explorer with a Pisces snub.",
                "url": "https://starcitizen.tools/Carrack",
            }
        ],
    )
    docs = search_docs(conn, "Carrack explorer", limit=5)
    assert docs
    assert "Carrack" in docs[0]["title"]
    conn.close()


def test_mcp_sc_search_docs(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    db = tmp_path / "t.sqlite"
    conn = connect(db)
    init_db(conn)
    sync_wiki(
        conn,
        documents=[
            {
                "title": "Carrack",
                "body": "Pathfinder explorer ship used for long range charting.",
                "url": "https://starcitizen.tools/Carrack",
            }
        ],
    )
    conn.commit()
    conn.close()
    monkeypatch.setenv("SC_SEARCH_DB", str(db))
    body = json.loads(mcp_server.handlers["sc_search_docs"]("pathfinder explorer"))
    assert body["ok"] is True
    assert body["data"]
