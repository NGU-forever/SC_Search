from pathlib import Path

from sc_search.ingest.adapters.aliases import apply_seed_aliases, load_seed_aliases
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import upsert_entity
from sc_search.paths import project_root
from sc_search.query.resolve import resolve_name


def test_resolve_hongfeng_to_red_wind(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    upsert_entity(
        conn,
        kind="faction",
        canonical_name="Red Wind Linehaul",
        game_version="4.10.3",
    )
    entries = load_seed_aliases(project_root() / "data" / "seed_aliases.json")
    hongfeng = [e for e in entries if e.get("alias") in ("红枫物流", "红枫")]
    assert len(hongfeng) == 2
    apply_seed_aliases(conn, hongfeng)
    conn.commit()
    matches = resolve_name(conn, "红枫物流")
    assert any(m["canonical_name"] == "Red Wind Linehaul" for m in matches)
    short = resolve_name(conn, "红枫")
    assert any(m["canonical_name"] == "Red Wind Linehaul" for m in short)
    conn.close()