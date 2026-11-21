from pathlib import Path

import httpx

from sc_search.ingest.adapters.uex import UexClient
from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import finish_run, start_run, upsert_alias, upsert_entity
from sc_search.query.answer import answer_question


TERMINALS = [
    {
        "id": 148,
        "name": "Astro Armada - Area 18",
        "code": "AA18",
        "game_version": "3.24.2",
    },
    {
        "id": 112,
        "name": "New Deal - Crusader Showroom - Orison",
        "code": "NDCO",
        "game_version": "3.24.2",
    },
]

VEHICLES = [
    {
        "id": 35,
        "name": "C2 Hercules Starlifter",
        "name_full": "Crusader C2 Hercules Starlifter",
        "slug": "c2-hercules",
        "game_version": "3.24.2",
    }
]

VEHICLE_PURCHASES = [
    {
        "id": 24,
        "id_vehicle": 35,
        "id_terminal": 148,
        "price_buy": 18900000,
        "vehicle_name": "C2 Hercules Starlifter",
        "terminal_name": "Astro Armada",
        "date_modified": 1794522145,
    },
    {
        "id": 110,
        "id_vehicle": 35,
        "id_terminal": 112,
        "price_buy": 18900000,
        "vehicle_name": "C2 Hercules Starlifter",
        "terminal_name": "New Deal Crusader",
        "date_modified": 1794598294,
    },
]


class FakeTransport(httpx.BaseTransport):
    def handle_request(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path.endswith("/terminals"):
            return httpx.Response(200, json={"status": "ok", "data": TERMINALS})
        if path.endswith("/commodities"):
            return httpx.Response(200, json={"status": "ok", "data": []})
        if path.endswith("/commodities_prices_all"):
            return httpx.Response(200, json={"status": "ok", "data": []})
        if path.endswith("/vehicles_purchases_prices_all"):
            return httpx.Response(200, json={"status": "ok", "data": VEHICLE_PURCHASES})
        if path.endswith("/vehicles"):
            return httpx.Response(200, json={"status": "ok", "data": VEHICLES})
        return httpx.Response(404, json={"status": "error", "message": path})


def test_uex_sync_links_vehicle_buy_to_existing_ship(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    scmdb = start_run(conn, "scmdb", "4.10.3-live.1")
    finish_run(conn, scmdb, status="ok", row_count=1, game_version="4.10.3-live.1")
    ship = upsert_entity(
        conn,
        kind="ship",
        canonical_name="C2 Hercules",
        game_version="scw-live",
        uuid="scw-c2",
    )
    upsert_alias(conn, ship, "大力神c2", "zh", "seed")
    upsert_alias(conn, ship, "Crusader C2 Hercules Starlifter", "en", "seed")
    conn.commit()

    http = httpx.Client(transport=FakeTransport(), base_url="https://example.test")
    client = UexClient(token="test-token", base_url="https://example.test/2.0", client=http)
    rows = client.sync(conn)
    conn.commit()
    assert rows > 0

    priced = conn.execute(
        """
        SELECT c.id AS ship_id, c.kind, t.canonical_name AS terminal, p.price, p.side
        FROM prices p
        JOIN entities c ON c.id = p.commodity_entity_id
        JOIN entities t ON t.id = p.terminal_entity_id
        WHERE c.kind = 'ship'
        ORDER BY t.canonical_name
        """
    ).fetchall()
    assert len(priced) == 2
    assert {r["ship_id"] for r in priced} == {ship}
    assert all(r["price"] == 18900000 for r in priced)
    assert all(r["side"] == "buy" for r in priced)
    client.close()
    conn.close()


def test_answer_ship_buy_uses_uex_vehicle_prices(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    scmdb = start_run(conn, "scmdb", "4.10.3-live.1")
    finish_run(conn, scmdb, status="ok", row_count=1, game_version="4.10.3-live.1")
    ship = upsert_entity(
        conn, kind="ship", canonical_name="C2 Hercules", game_version="scw-live"
    )
    sibling = upsert_entity(
        conn,
        kind="ship",
        canonical_name="Crusader C2 Hercules Starlifter",
        game_version="scw-live",
    )
    upsert_alias(conn, ship, "大力神c2", "zh", "seed")
    upsert_alias(conn, sibling, "大力神c2", "zh", "seed")
    term = upsert_entity(
        conn,
        kind="terminal",
        canonical_name="Astro Armada - Area 18",
        game_version="3.24.2",
    )
    from sc_search.ingest.normalize import replace_price

    run = start_run(conn, "uex", "3.24.2")
    # Price attached to sibling spelling (common after UEX name_full match).
    replace_price(
        conn,
        commodity_id=sibling,
        terminal_id=term,
        side="buy",
        price=18900000,
        source="uex",
        game_version="3.24.2",
        ingest_run_id=run,
    )
    finish_run(conn, run, status="ok", row_count=1, game_version="3.24.2")
    conn.commit()

    r = answer_question(conn, "大力神c2在哪里买")
    acq = r["evidence"].get("acquisition") or {}
    assert acq.get("shop_buyable") == "yes"
    assert r["evidence"].get("prices")
    assert "18900000" in r["answer_draft"] or "18,900,000" in r["answer_draft"]
    assert "Astro Armada" in r["answer_draft"]
    assert "【供 Agent 深度整理】" in r["answer_draft"]
    assert r["evidence"].get("reasoning_hint")
    assert "无法确认哪里买" not in r["answer_draft"]
    conn.close()
