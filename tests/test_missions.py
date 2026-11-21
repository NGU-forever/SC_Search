from __future__ import annotations

from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import start_run, upsert_alias, upsert_entity, upsert_fact
from sc_search.query.missions import list_missions


def _seed_missions(conn) -> None:
    run_id = start_run(conn, "scmdb", "4.10.3")
    faction_id = upsert_entity(
        conn,
        kind="faction",
        canonical_name="Red Wind Linehaul",
        game_version="4.10.3",
    )
    upsert_alias(conn, faction_id, "Red Wind Linehaul", "en", "scmdb")

    def mission(
        canonical: str,
        title: str,
        contractor: str,
        reward: float,
        systems: str,
        mission_type: str,
    ) -> None:
        eid = upsert_entity(
            conn,
            kind="mission",
            canonical_name=canonical,
            game_version="4.10.3",
        )
        upsert_alias(conn, eid, canonical, "en", "scmdb")
        if title != canonical:
            upsert_alias(conn, eid, title, "en", "scmdb")
        upsert_fact(
            conn,
            entity_id=eid,
            key="contractor",
            value_text=contractor,
            source="scmdb",
            game_version="4.10.3",
            ingest_run_id=run_id,
        )
        upsert_fact(
            conn,
            entity_id=eid,
            key="reward_uec",
            value_num=reward,
            unit="aUEC",
            source="scmdb",
            game_version="4.10.3",
            ingest_run_id=run_id,
        )
        upsert_fact(
            conn,
            entity_id=eid,
            key="systems",
            value_text=systems,
            source="scmdb",
            game_version="4.10.3",
            ingest_run_id=run_id,
        )
        upsert_fact(
            conn,
            entity_id=eid,
            key="mission_type",
            value_text=mission_type,
            source="scmdb",
            game_version="4.10.3",
            ingest_run_id=run_id,
        )

    mission(
        "RW_Junior_Haul_01",
        "Junior Level Contract: Haul Cargo",
        "Red Wind Linehaul",
        12_000,
        "Stanton",
        "Hauling",
    )
    mission(
        "RW_Junior_Haul_02",
        "Junior Level Contract: Pick Up",
        "Red Wind Linehaul",
        8_000,
        "Stanton",
        "Hauling",
    )
    mission(
        "RW_Senior_Patrol",
        "Senior Level Contract: Patrol",
        "Red Wind Linehaul",
        50_000,
        "Pyro",
        "Mercenary",
    )
    mission(
        "Foxwell_Test_Mission",
        "Orange Level Contract: Test Patrol",
        "Foxwell Enforcement",
        5_000,
        "Pyro",
        "Mercenary",
    )
    conn.commit()


@pytest.fixture
def conn_with_missions(tmp_path: Path):
    db = tmp_path / "t.sqlite"
    c = connect(db)
    init_db(c)
    _seed_missions(c)
    yield c
    c.close()


def test_list_missions_red_wind_junior(conn_with_missions) -> None:
    rows = list_missions(
        conn_with_missions,
        faction="Red Wind Linehaul",
        rank_keyword="Junior",
        limit=5,
    )
    assert rows
    assert all("Red Wind" in (r.get("contractor") or "") for r in rows)
    assert all(
        "Junior" in (r.get("title") or "") or "Junior" in (r.get("canonical_name") or "")
        for r in rows
    )
    assert rows[0]["reward_uec"] >= rows[-1]["reward_uec"]


def test_list_missions_system_filter(conn_with_missions) -> None:
    rows = list_missions(conn_with_missions, system="Stanton", limit=20)
    assert rows
    assert all("Stanton" in (r.get("systems") or "") for r in rows)


def test_list_missions_empty_faction(conn_with_missions) -> None:
    rows = list_missions(conn_with_missions, faction="Nonexistent Faction XYZ", limit=5)
    assert rows == []