from __future__ import annotations

import os
import sqlite3
from typing import Any

import httpx
from dotenv import load_dotenv

from sc_search.ingest.normalize import (
    finish_run,
    replace_price,
    start_run,
    upsert_alias,
    upsert_entity,
)
from sc_search.paths import project_root

USER_AGENT = "SC_Search-ingest/0.1"
DEFAULT_BASE = "https://api.uexcorp.space/2.0"


class UexError(RuntimeError):
    pass


class UexClient:
    def __init__(
        self,
        token: str | None = None,
        base_url: str = DEFAULT_BASE,
        client: httpx.Client | None = None,
    ) -> None:
        load_dotenv(project_root() / "config" / ".env")
        self.token = token if token is not None else os.environ.get("UEX_API_TOKEN", "").strip()
        if not self.token:
            raise UexError("UEX_API_TOKEN missing; set it in config/.env")
        self.base_url = base_url.rstrip("/")
        self._client = client or httpx.Client(
            timeout=60.0,
            headers={
                "Authorization": f"Bearer {self.token}",
                "User-Agent": USER_AGENT,
                "Accept": "application/json",
            },
        )
        self._owns_client = client is None

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def _get(self, path: str, params: dict[str, Any] | None = None) -> Any:
        url = f"{self.base_url}/{path.lstrip('/')}"
        resp = self._client.get(url, params=params)
        if resp.status_code == 429:
            raise UexError("UEX rate limit reached")
        try:
            body = resp.json()
        except ValueError as exc:
            raise UexError(f"UEX non-JSON response HTTP {resp.status_code}") from exc
        if resp.status_code >= 400:
            raise UexError(f"UEX HTTP {resp.status_code}: {body}")
        if isinstance(body, dict) and body.get("status") not in (None, "ok"):
            raise UexError(f"UEX status={body.get('status')}: {body}")
        if isinstance(body, dict) and "data" in body:
            return body["data"]
        return body

    def fetch_terminals(self) -> list[dict[str, Any]]:
        data = self._get("terminals")
        if not isinstance(data, list):
            raise UexError("terminals response was not a list")
        return data

    def fetch_commodities(self) -> list[dict[str, Any]]:
        data = self._get("commodities")
        if not isinstance(data, list):
            raise UexError("commodities response was not a list")
        return data

    def fetch_commodities_prices_all(self) -> list[dict[str, Any]]:
        data = self._get("commodities_prices_all")
        if not isinstance(data, list):
            raise UexError("commodities_prices_all response was not a list")
        return data

    def fetch_vehicles(self) -> list[dict[str, Any]]:
        data = self._get("vehicles")
        if not isinstance(data, list):
            raise UexError("vehicles response was not a list")
        return data

    def fetch_vehicles_purchases_prices_all(self) -> list[dict[str, Any]]:
        data = self._get("vehicles_purchases_prices_all")
        if not isinstance(data, list):
            raise UexError("vehicles_purchases_prices_all response was not a list")
        return data

    def sync(self, conn: sqlite3.Connection) -> int:
        run_id = start_run(conn, "uex")
        try:
            terminals = self.fetch_terminals()
            commodities = self.fetch_commodities()
            prices = self.fetch_commodities_prices_all()
            vehicles = self.fetch_vehicles()
            vehicle_purchases = self.fetch_vehicles_purchases_prices_all()
            game_version = (
                _pick_game_version(
                    terminals, commodities, prices, vehicles, vehicle_purchases
                )
                or "unknown"
            )
            rows = _apply_uex_payload(
                conn,
                run_id=run_id,
                game_version=game_version,
                terminals=terminals,
                commodities=commodities,
                prices=prices,
            )
            rows += _apply_uex_vehicle_purchases(
                conn,
                run_id=run_id,
                game_version=game_version,
                terminals=terminals,
                vehicles=vehicles,
                purchases=vehicle_purchases,
            )
            finish_run(
                conn,
                run_id,
                status="ok",
                row_count=rows,
                game_version=game_version,
            )
            return rows
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise


def _pick_game_version(*groups: list[dict[str, Any]]) -> str | None:
    for group in groups:
        for item in group:
            ver = item.get("game_version")
            if ver:
                return str(ver)
    return None


def _apply_uex_payload(
    conn: sqlite3.Connection,
    *,
    run_id: int,
    game_version: str,
    terminals: list[dict[str, Any]],
    commodities: list[dict[str, Any]],
    prices: list[dict[str, Any]],
) -> int:
    terminal_ids: dict[int, int] = {}
    commodity_ids: dict[int, int] = {}
    count = 0

    for t in terminals:
        uex_id = t.get("id")
        name = t.get("name") or t.get("displayname") or t.get("fullname")
        if uex_id is None or not name:
            continue
        entity_id = upsert_entity(
            conn,
            kind="terminal",
            canonical_name=str(name),
            game_version=game_version,
            slug=str(t.get("code") or "") or None,
            uuid=f"uex-terminal-{uex_id}",
        )
        terminal_ids[int(uex_id)] = entity_id
        upsert_alias(conn, entity_id, str(name), "en", "uex")
        count += 1

    for c in commodities:
        uex_id = c.get("id")
        name = c.get("name")
        if uex_id is None or not name:
            continue
        entity_id = upsert_entity(
            conn,
            kind="commodity",
            canonical_name=str(name),
            game_version=game_version,
            slug=str(c.get("slug") or "") or None,
            uuid=str(c.get("uuid") or f"uex-commodity-{uex_id}"),
        )
        commodity_ids[int(uex_id)] = entity_id
        upsert_alias(conn, entity_id, str(name), "en", "uex")
        count += 1

    for p in prices:
        commodity_uex = p.get("id_commodity")
        terminal_uex = p.get("id_terminal")
        if commodity_uex is None or terminal_uex is None:
            continue
        commodity_id = commodity_ids.get(int(commodity_uex))
        terminal_id = terminal_ids.get(int(terminal_uex))
        if commodity_id is None or terminal_id is None:
            # Ensure entities exist from price row names when list endpoints omitted them
            cname = p.get("commodity_name")
            tname = p.get("terminal_name") or p.get("terminal_displayname")
            if commodity_id is None and cname:
                commodity_id = upsert_entity(
                    conn,
                    kind="commodity",
                    canonical_name=str(cname),
                    game_version=game_version,
                    uuid=f"uex-commodity-{commodity_uex}",
                )
                commodity_ids[int(commodity_uex)] = commodity_id
                upsert_alias(conn, commodity_id, str(cname), "en", "uex")
            if terminal_id is None and tname:
                terminal_id = upsert_entity(
                    conn,
                    kind="terminal",
                    canonical_name=str(tname),
                    game_version=game_version,
                    uuid=f"uex-terminal-{terminal_uex}",
                )
                terminal_ids[int(terminal_uex)] = terminal_id
                upsert_alias(conn, terminal_id, str(tname), "en", "uex")
        if commodity_id is None or terminal_id is None:
            continue

        reported = p.get("date_modified") or p.get("date_added")
        reported_at = str(reported) if reported is not None else None

        price_buy = p.get("price_buy")
        if price_buy is not None and float(price_buy) > 0:
            replace_price(
                conn,
                commodity_id=commodity_id,
                terminal_id=terminal_id,
                side="buy",
                price=float(price_buy),
                scu_available=_maybe_float(p.get("scu_buy") or p.get("scu_buy_avg")),
                reported_at=reported_at,
                source="uex",
                game_version=game_version,
                ingest_run_id=run_id,
            )
            count += 1

        price_sell = p.get("price_sell")
        if price_sell is not None and float(price_sell) > 0:
            replace_price(
                conn,
                commodity_id=commodity_id,
                terminal_id=terminal_id,
                side="sell",
                price=float(price_sell),
                scu_available=_maybe_float(p.get("scu_sell") or p.get("scu_sell_avg")),
                reported_at=reported_at,
                source="uex",
                game_version=game_version,
                ingest_run_id=run_id,
            )
            count += 1

    return count


def _find_existing_ship_id(conn: sqlite3.Connection, *names: str) -> int | None:
    candidates = [n.strip() for n in names if n and str(n).strip()]
    if not candidates:
        return None
    for name in candidates:
        row = conn.execute(
            """
            SELECT e.id
            FROM entities e
            WHERE e.kind = 'ship' AND e.canonical_name = ?
            ORDER BY CASE WHEN e.game_version LIKE 'scw%' THEN 0 ELSE 1 END, e.id
            LIMIT 1
            """,
            (name,),
        ).fetchone()
        if row:
            return int(row["id"])
        row = conn.execute(
            """
            SELECT e.id
            FROM aliases a
            JOIN entities e ON e.id = a.entity_id
            WHERE e.kind = 'ship' AND a.alias = ? COLLATE NOCASE
            ORDER BY CASE WHEN e.game_version LIKE 'scw%' THEN 0 ELSE 1 END, e.id
            LIMIT 1
            """,
            (name,),
        ).fetchone()
        if row:
            return int(row["id"])
    return None


def _apply_uex_vehicle_purchases(
    conn: sqlite3.Connection,
    *,
    run_id: int,
    game_version: str,
    terminals: list[dict[str, Any]],
    vehicles: list[dict[str, Any]],
    purchases: list[dict[str, Any]],
) -> int:
    terminal_ids: dict[int, int] = {}
    vehicle_ids: dict[int, int] = {}
    count = 0

    for t in terminals:
        uex_id = t.get("id")
        name = t.get("name") or t.get("displayname") or t.get("nickname")
        if uex_id is None or not name:
            continue
        entity_id = upsert_entity(
            conn,
            kind="terminal",
            canonical_name=str(name),
            game_version=game_version,
            slug=str(t.get("code") or "") or None,
            uuid=f"uex-terminal-{uex_id}",
        )
        terminal_ids[int(uex_id)] = entity_id
        upsert_alias(conn, entity_id, str(name), "en", "uex")

    for v in vehicles:
        uex_id = v.get("id")
        name = v.get("name")
        name_full = v.get("name_full") or name
        if uex_id is None or not name:
            continue
        ship_id = _find_existing_ship_id(conn, str(name_full), str(name))
        if ship_id is None:
            ship_id = upsert_entity(
                conn,
                kind="ship",
                canonical_name=str(name_full or name),
                game_version=game_version,
                slug=str(v.get("slug") or "") or None,
                uuid=f"uex-vehicle-{uex_id}",
            )
            count += 1
        vehicle_ids[int(uex_id)] = ship_id
        for alias in {str(name), str(name_full)}:
            if alias:
                upsert_alias(conn, ship_id, alias, "en", "uex")

    for p in purchases:
        vehicle_uex = p.get("id_vehicle")
        terminal_uex = p.get("id_terminal")
        price_buy = p.get("price_buy")
        if vehicle_uex is None or terminal_uex is None or price_buy is None:
            continue
        if float(price_buy) <= 0:
            continue
        ship_id = vehicle_ids.get(int(vehicle_uex))
        terminal_id = terminal_ids.get(int(terminal_uex))
        if ship_id is None:
            vname = p.get("vehicle_name")
            if vname:
                ship_id = _find_existing_ship_id(conn, str(vname))
                if ship_id is None:
                    ship_id = upsert_entity(
                        conn,
                        kind="ship",
                        canonical_name=str(vname),
                        game_version=game_version,
                        uuid=f"uex-vehicle-{vehicle_uex}",
                    )
                vehicle_ids[int(vehicle_uex)] = ship_id
                upsert_alias(conn, ship_id, str(vname), "en", "uex")
        if terminal_id is None:
            tname = p.get("terminal_name")
            if tname:
                terminal_id = upsert_entity(
                    conn,
                    kind="terminal",
                    canonical_name=str(tname),
                    game_version=game_version,
                    uuid=f"uex-terminal-{terminal_uex}",
                )
                terminal_ids[int(terminal_uex)] = terminal_id
                upsert_alias(conn, terminal_id, str(tname), "en", "uex")
        if ship_id is None or terminal_id is None:
            continue
        reported = p.get("date_modified") or p.get("date_added")
        reported_at = str(reported) if reported is not None else None
        replace_price(
            conn,
            commodity_id=ship_id,
            terminal_id=terminal_id,
            side="buy",
            price=float(price_buy),
            reported_at=reported_at,
            source="uex",
            game_version=game_version,
            ingest_run_id=run_id,
        )
        count += 1

    return count


def _maybe_float(value: Any) -> float | None:
    if value is None or value == "":
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def sync_uex(conn: sqlite3.Connection, client: UexClient | None = None) -> int:
    owned = client is None
    client = client or UexClient()
    try:
        return client.sync(conn)
    finally:
        if owned:
            client.close()
