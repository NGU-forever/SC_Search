from pathlib import Path

import httpx

from sc_search.ingest.adapters.uex import UexClient
from sc_search.ingest.db import connect, init_db


TERMINALS = [
    {
        "id": 1,
        "name": "Area18",
        "code": "A18",
        "game_version": "4.10.3",
    }
]

COMMODITIES = [
    {
        "id": 10,
        "name": "Gold",
        "slug": "gold",
        "uuid": "uuid-gold",
        "game_version": "4.10.3",
    }
]

PRICES = [
    {
        "id_commodity": 10,
        "id_terminal": 1,
        "commodity_name": "Gold",
        "terminal_name": "Area18",
        "price_buy": 50.0,
        "price_sell": 200.0,
        "scu_buy": 10,
        "scu_sell": 5,
        "date_modified": 1700000000,
        "game_version": "4.10.3",
    }
]


class FakeTransport(httpx.BaseTransport):
    def handle_request(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path.endswith("/terminals"):
            return httpx.Response(200, json={"status": "ok", "data": TERMINALS})
        if path.endswith("/commodities"):
            return httpx.Response(200, json={"status": "ok", "data": COMMODITIES})
        if path.endswith("/commodities_prices_all"):
            return httpx.Response(200, json={"status": "ok", "data": PRICES})
        if path.endswith("/vehicles_purchases_prices_all"):
            return httpx.Response(200, json={"status": "ok", "data": []})
        if path.endswith("/vehicles"):
            return httpx.Response(200, json={"status": "ok", "data": []})
        return httpx.Response(404, json={"status": "error", "message": path})


def test_uex_sync_from_fixture(tmp_path: Path) -> None:
    conn = connect(tmp_path / "t.sqlite")
    init_db(conn)
    http = httpx.Client(transport=FakeTransport(), base_url="https://example.test")
    client = UexClient(token="test-token", base_url="https://example.test/2.0", client=http)
    rows = client.sync(conn)
    conn.commit()
    assert rows > 0
    price_count = conn.execute("SELECT COUNT(*) AS c FROM prices").fetchone()["c"]
    assert price_count == 2
    sell = conn.execute(
        "SELECT price FROM prices WHERE side = 'sell'"
    ).fetchone()["price"]
    assert sell == 200.0
    run = conn.execute(
        "SELECT status, game_version FROM ingest_runs ORDER BY id DESC LIMIT 1"
    ).fetchone()
    assert run["status"] == "ok"
    assert run["game_version"] == "4.10.3"
    client.close()
    conn.close()
