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


def test_seed_has_citizenwiki_chinese_ship_names() -> None:
    entries = load_seed_aliases(project_root() / "data" / "seed_aliases.json")
    by_alias = {e["alias"]: e for e in entries}
    assert by_alias["锤头鲨"]["target"] == "Hammerhead"
    assert by_alias["远行者"]["target"] == "Starfarer"
    assert by_alias["珀修斯"]["target"] == "Perseus"
    assert by_alias["采矿车"]["target"] == "ROC"
    assert by_alias["海妖"]["target"] == "Kraken"
    # over-broad component slang must stay out of seed
    assert "发电机" not in by_alias
    assert "护盾" not in by_alias


def test_resolve_chuiyusha_and_yuanzingzhe(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    upsert_entity(conn, kind="ship", canonical_name="Hammerhead", game_version="4.10.3")
    upsert_entity(conn, kind="ship", canonical_name="Starfarer", game_version="4.10.3")
    entries = load_seed_aliases(project_root() / "data" / "seed_aliases.json")
    subset = [e for e in entries if e.get("alias") in ("锤头鲨", "远行者")]
    apply_seed_aliases(conn, subset)
    conn.commit()
    assert any(m["canonical_name"] == "Hammerhead" for m in resolve_name(conn, "锤头鲨"))
    assert any(m["canonical_name"] == "Starfarer" for m in resolve_name(conn, "远行者"))
    conn.close()