from pathlib import Path

from sc_search.ingest.db import connect, init_db


def test_init_creates_core_tables(tmp_path: Path) -> None:
    db = tmp_path / "t.sqlite"
    conn = connect(db)
    init_db(conn)
    names = {
        r[0]
        for r in conn.execute("SELECT name FROM sqlite_master WHERE type='table'")
    }
    assert {
        "ingest_runs",
        "entities",
        "aliases",
        "facts",
        "prices",
        "relations",
        "documents",
    } <= names
    conn.close()
