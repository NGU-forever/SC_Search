from pathlib import Path

from sc_search.ingest.adapters.localization import apply_localization_maps, parse_ini, sync_localization
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run, upsert_alias, upsert_entity
from sc_search.query.resolve import resolve_name


def test_parse_ini_bom() -> None:
    text = "\ufeffvehicle_nameANVL_Carrack_short=Carrack\n"
    assert parse_ini(text)["vehicle_nameANVL_Carrack_short"] == "Carrack"


def test_apply_localization_maps(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    ship = upsert_entity(
        conn,
        kind="ship",
        canonical_name="Anvil Carrack",
        game_version="scw-live",
        uuid="scw-ship-carrack",
    )
    upsert_alias(conn, ship, "Carrack", "en", "test")
    gold = upsert_entity(
        conn,
        kind="commodity",
        canonical_name="Gold",
        game_version="uex",
        uuid="uex-commodity-gold",
    )
    upsert_alias(conn, gold, "Gold", "en", "test")
    conn.commit()

    en_map = {
        "vehicle_nameANVL_Carrack": "Anvil Carrack",
        "vehicle_nameANVL_Carrack_short": "Carrack",
        "vehicle_DescANVL_Carrack": "Manufacturer: Anvil\nFocus: Expedition",
        "items_commodities_gold": "Gold",
        "Stanton4_NewBabbage": "New Babbage",
    }
    zh_map = {
        "vehicle_nameANVL_Carrack": "铁砧 克拉克",
        "vehicle_nameANVL_Carrack_short": "克拉克",
        "vehicle_DescANVL_Carrack": "制造商：铁砧航天\n定位：远征\n要说探险哪家强。",
        "items_commodities_gold": "金",
        "Stanton4_NewBabbage": "新巴贝奇",
    }
    run_id = start_run(conn, "localization")
    rows = apply_localization_maps(conn, en_map, zh_map, run_id=run_id)
    conn.commit()
    assert rows > 0

    hits = resolve_name(conn, "克拉克")
    assert hits
    assert "Carrack" in hits[0]["canonical_name"] or hits[0]["canonical_name"] == "Anvil Carrack"

    gold_hits = resolve_name(conn, "金")
    assert gold_hits
    assert gold_hits[0]["canonical_name"] == "Gold"

    fact = conn.execute(
        """
        SELECT value_text FROM facts
        WHERE entity_id = ? AND key = 'description_zh'
        """,
        (ship,),
    ).fetchone()
    assert fact is not None
    assert "铁砧" in fact["value_text"]
    conn.close()


def test_sync_localization_with_maps(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    rows = sync_localization(
        conn,
        en_map={"vehicle_nameDRAK_Cutlass_Black_short": "Cutlass Black"},
        zh_map={"vehicle_nameDRAK_Cutlass_Black_short": "黑弯刀"},
    )
    conn.commit()
    assert rows >= 1
    assert resolve_name(conn, "黑弯刀")
    run = conn.execute(
        "SELECT status, notes FROM ingest_runs WHERE source = 'localization' ORDER BY id DESC LIMIT 1"
    ).fetchone()
    assert run["status"] == "ok"
    assert "CC BY-NC-SA" in (run["notes"] or "")
    conn.close()
