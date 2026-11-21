from __future__ import annotations

import json
import sqlite3
import time
from typing import Any

import httpx

from sc_search.ingest.normalize import (
    finish_run,
    insert_relation,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)

USER_AGENT = "SC_Search-ingest/0.1"
BASE = "https://api.star-citizen.wiki"
SOURCE = "scw_blueprint"
GAME_VERSION = "scw-blueprint-live"


class ScwBlueprintError(RuntimeError):
    pass


class ScwBlueprintClient:
    def __init__(self, client: httpx.Client | None = None, delay_s: float = 0.05) -> None:
        self._client = client or httpx.Client(
            timeout=60.0,
            headers={"User-Agent": USER_AGENT, "Accept": "application/json"},
            follow_redirects=True,
        )
        self._owns = client is None
        self.delay_s = delay_s

    def close(self) -> None:
        if self._owns:
            self._client.close()

    def iter_blueprints(self, page_size: int = 50) -> list[dict[str, Any]]:
        page = 1
        out: list[dict[str, Any]] = []
        while True:
            resp = self._client.get(
                f"{BASE}/api/blueprints",
                params={"page[size]": page_size, "page[number]": page},
            )
            if resp.status_code >= 400:
                raise ScwBlueprintError(f"SCW blueprints HTTP {resp.status_code}: {resp.text[:200]}")
            body = resp.json()
            chunk = body.get("data") or []
            if not isinstance(chunk, list):
                raise ScwBlueprintError("SCW blueprints data was not a list")
            out.extend(chunk)
            meta = body.get("meta") or {}
            last = int(meta.get("last_page") or page)
            if page >= last or not chunk:
                break
            page += 1
            if self.delay_s:
                time.sleep(self.delay_s)
        return out

    def sync(self, conn: sqlite3.Connection, *, rows: list[dict[str, Any]] | None = None) -> int:
        run_id = start_run(conn, SOURCE, GAME_VERSION)
        try:
            payload = rows if rows is not None else self.iter_blueprints()
            n = apply_scw_blueprints(conn, payload, run_id=run_id)
            finish_run(conn, run_id, status="ok", row_count=n, game_version=GAME_VERSION)
            return n
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise


def _clear_scw_blueprint(conn: sqlite3.Connection) -> None:
    conn.execute("DELETE FROM facts WHERE source = ?", (SOURCE,))
    conn.execute("DELETE FROM relations WHERE source = ?", (SOURCE,))


def _find_blueprint_id(conn: sqlite3.Connection, api_uuid: str, output_name: str) -> int | None:
    row = conn.execute(
        """
        SELECT id FROM entities
        WHERE kind = 'blueprint' AND uuid = ? COLLATE NOCASE
        LIMIT 1
        """,
        (f"scmdb-bp-{api_uuid}",),
    ).fetchone()
    if row:
        return int(row["id"])
    row = conn.execute(
        """
        SELECT id FROM entities
        WHERE kind = 'blueprint' AND uuid = ? COLLATE NOCASE
        LIMIT 1
        """,
        (f"scw-bp-{api_uuid}",),
    ).fetchone()
    if row:
        return int(row["id"])
    row = conn.execute(
        """
        SELECT id FROM entities
        WHERE kind = 'blueprint' AND canonical_name = ? COLLATE NOCASE
        ORDER BY id
        LIMIT 1
        """,
        (output_name,),
    ).fetchone()
    if row:
        return int(row["id"])
    return None


def apply_scw_blueprints(
    conn: sqlite3.Connection,
    blueprints: list[dict[str, Any]],
    *,
    run_id: int,
) -> int:
    _clear_scw_blueprint(conn)
    count = 0
    for bp in blueprints:
        if not isinstance(bp, dict):
            continue
        api_uuid = str(bp.get("uuid") or "").strip()
        output_name = str(bp.get("output_name") or "").strip()
        if not api_uuid or not output_name:
            continue

        entity_id = _find_blueprint_id(conn, api_uuid, output_name)
        if entity_id is None:
            entity_id = upsert_entity(
                conn,
                kind="blueprint",
                canonical_name=output_name,
                game_version=GAME_VERSION,
                uuid=f"scw-bp-{api_uuid}",
            )
            upsert_alias(conn, entity_id, output_name, "en", SOURCE)

        if bp.get("craft_time_seconds") is not None:
            try:
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key="craft_time_seconds",
                    value_num=float(bp["craft_time_seconds"]),
                    unit="s",
                    source=SOURCE,
                    game_version=GAME_VERSION,
                    ingest_run_id=run_id,
                )
            except (TypeError, ValueError):
                pass
        label = bp.get("craft_time_label")
        if label:
            upsert_fact(
                conn,
                entity_id=entity_id,
                key="craft_time_label",
                value_text=str(label),
                source=SOURCE,
                game_version=GAME_VERSION,
                ingest_run_id=run_id,
            )

        for ing in bp.get("ingredients") or []:
            if not isinstance(ing, dict):
                continue
            mat_name = str(ing.get("name") or "").strip()
            if not mat_name:
                continue
            kind = str(ing.get("kind") or "resource").lower()
            entity_kind = "commodity" if kind == "resource" else "item"
            mat_uuid = str(ing.get("resource_type_uuid") or ing.get("item_uuid") or "") or None
            mat_id = upsert_entity(
                conn,
                kind=entity_kind,
                canonical_name=mat_name,
                game_version=GAME_VERSION,
                uuid=f"scw-mat-{mat_uuid}" if mat_uuid else None,
            )
            upsert_alias(conn, mat_id, mat_name, "en", SOURCE)

            qty: float | None = None
            unit = "count"
            if ing.get("quantity_scu") is not None:
                try:
                    qty = float(ing["quantity_scu"])
                    unit = "SCU"
                except (TypeError, ValueError):
                    qty = None
            elif ing.get("quantity") is not None:
                try:
                    qty = float(ing["quantity"])
                    unit = "count"
                except (TypeError, ValueError):
                    qty = None

            insert_relation(
                conn,
                from_id=entity_id,
                to_id=mat_id,
                rel_type="blueprint_requires",
                source=SOURCE,
                game_version=GAME_VERSION,
                qty=qty,
                meta_json=json.dumps(
                    {"unit": unit, "ingredient_kind": kind},
                    ensure_ascii=False,
                ),
            )
        count += 1
    return count


def sync_scw_blueprints(
    conn: sqlite3.Connection,
    *,
    client: ScwBlueprintClient | None = None,
    rows: list[dict[str, Any]] | None = None,
) -> int:
    owned = client is None
    client = client or ScwBlueprintClient()
    try:
        return client.sync(conn, rows=rows)
    finally:
        if owned:
            client.close()
