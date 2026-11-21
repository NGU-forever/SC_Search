from pathlib import Path

from sc_search.ingest.adapters.scw_blueprints import apply_scw_blueprints, sync_scw_blueprints
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import upsert_alias, upsert_entity


def test_apply_scw_blueprints_links_existing_scmdb_blueprint(tmp_path: Path):
    db = tmp_path / "bp.sqlite"
    conn = connect(db)
    init_db(conn)
    bp = upsert_entity(
        conn,
        kind="blueprint",
        canonical_name="CQ7 Rifle",
        game_version="4.10.3-live",
        uuid="scmdb-bp-1a85280e-7b8f-4486-a563-17cd2549d268",
    )
    upsert_alias(conn, bp, "CQ7 Rifle", "en", "scmdb")
    conn.commit()

    sample = [
        {
            "uuid": "1a85280e-7b8f-4486-a563-17cd2549d268",
            "output_name": "CQ7 Rifle",
            "craft_time_seconds": 150,
            "craft_time_label": "2 minutes 30 seconds",
            "ingredients": [
                {
                    "name": "Aluminum",
                    "kind": "resource",
                    "resource_type_uuid": "aaa",
                    "quantity_scu": 0.06,
                },
                {
                    "name": "Iron",
                    "kind": "resource",
                    "resource_type_uuid": "bbb",
                    "quantity_scu": 0.01,
                },
            ],
        }
    ]
    n = sync_scw_blueprints(conn, rows=sample)
    assert n == 1
    mats = conn.execute(
        """
        SELECT te.canonical_name, rel.qty
        FROM relations rel
        JOIN entities te ON te.id = rel.to_id
        WHERE rel.from_id = ? AND rel.rel_type = 'blueprint_requires'
        ORDER BY te.canonical_name
        """,
        (bp,),
    ).fetchall()
    assert len(mats) == 2
    assert mats[0]["canonical_name"] == "Aluminum"
    assert float(mats[0]["qty"]) == 0.06
    fact = conn.execute(
        "SELECT value_num FROM facts WHERE entity_id = ? AND key = 'craft_time_seconds'",
        (bp,),
    ).fetchone()
    assert float(fact["value_num"]) == 150.0
    conn.close()
