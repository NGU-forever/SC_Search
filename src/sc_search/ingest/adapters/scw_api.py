from __future__ import annotations

import json
import sqlite3
import time
from typing import Any

import httpx

from sc_search.ingest.normalize import (
    clear_source_facts_relations,
    finish_run,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)

USER_AGENT = "SC_Search-ingest/0.1"
BASE = "https://api.star-citizen.wiki"


class ScwError(RuntimeError):
    pass


def _localized(value: Any, lang: str = "en_EN") -> str | None:
    if value is None:
        return None
    if isinstance(value, dict):
        if value.get(lang):
            return str(value[lang])
        if value.get("en_EN"):
            return str(value["en_EN"])
        for v in value.values():
            if v:
                return str(v)
        return None
    return str(value)


class ScwClient:
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

    def iter_vehicles(self, page_size: int = 50) -> list[dict[str, Any]]:
        return self._paginate("/api/vehicles", page_size=page_size)

    def _paginate(self, path: str, page_size: int = 50) -> list[dict[str, Any]]:
        page = 1
        out: list[dict[str, Any]] = []
        while True:
            resp = self._client.get(
                f"{BASE}{path}",
                params={"page[size]": page_size, "page[number]": page},
            )
            if resp.status_code >= 400:
                raise ScwError(f"SCW HTTP {resp.status_code}: {resp.text[:200]}")
            body = resp.json()
            chunk = body.get("data") or []
            if not isinstance(chunk, list):
                raise ScwError("SCW data was not a list")
            out.extend(chunk)
            meta = body.get("meta") or {}
            last = int(meta.get("last_page") or page)
            if page >= last or not chunk:
                break
            page += 1
            if self.delay_s:
                time.sleep(self.delay_s)
        return out

    def sync(self, conn: sqlite3.Connection, *, vehicles: list[dict[str, Any]] | None = None) -> int:
        run_id = start_run(conn, "scw_api")
        try:
            vehicles = vehicles if vehicles is not None else self.iter_vehicles()
            game_version = "scw-live"
            rows = apply_scw_vehicles(conn, vehicles, run_id=run_id, game_version=game_version)
            finish_run(conn, run_id, status="ok", row_count=rows, game_version=game_version)
            return rows
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise


def apply_scw_vehicles(
    conn: sqlite3.Connection,
    vehicles: list[dict[str, Any]],
    *,
    run_id: int,
    game_version: str,
) -> int:
    clear_source_facts_relations(conn, "scw_api", game_version)
    count = 0
    for v in vehicles:
        if not isinstance(v, dict):
            continue
        name = str(v.get("name") or v.get("game_name") or v.get("slug") or "")
        uuid = str(v.get("uuid") or "")
        if not name or not uuid:
            continue
        # Prefer game_name when present for uniqueness across variants sharing short name
        canonical = str(v.get("game_name") or name)
        entity_id = upsert_entity(
            conn,
            kind="ship",
            canonical_name=canonical,
            game_version=game_version,
            slug=str(v.get("slug") or "") or None,
            uuid=f"scw-ship-{uuid}",
        )
        upsert_alias(conn, entity_id, name, "en", "scw_api")
        if canonical != name:
            upsert_alias(conn, entity_id, canonical, "en", "scw_api")
        if v.get("slug"):
            upsert_alias(conn, entity_id, str(v["slug"]), "en", "scw_api")

        mfr = v.get("manufacturer") or {}
        if isinstance(mfr, dict) and mfr.get("name"):
            upsert_fact(
                conn,
                entity_id=entity_id,
                key="manufacturer",
                value_text=str(mfr["name"]),
                source="scw_api",
                game_version=game_version,
                ingest_run_id=run_id,
            )
            if mfr.get("code"):
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key="manufacturer_code",
                    value_text=str(mfr["code"]),
                    source="scw_api",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )

        if v.get("cargo_capacity") is not None:
            upsert_fact(
                conn,
                entity_id=entity_id,
                key="cargo_scu",
                value_num=float(v["cargo_capacity"]),
                unit="SCU",
                source="scw_api",
                game_version=game_version,
                ingest_run_id=run_id,
            )
        for key, fact_key in [
            ("size", "size"),
            ("type", "type"),
            ("production_status", "production_status"),
            ("career", "career"),
        ]:
            en = _localized(v.get(key), "en_EN")
            zh = _localized(v.get(key), "zh_CN")
            if en:
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key=fact_key,
                    value_text=en,
                    source="scw_api",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )
            if zh and zh != en:
                upsert_alias(conn, entity_id, zh, "zh", "scw_api")

        foci = v.get("foci") or []
        if isinstance(foci, list) and foci:
            labels_en = []
            for focus in foci:
                en = _localized(focus, "en_EN")
                zh = _localized(focus, "zh_CN")
                if en:
                    labels_en.append(en)
                if zh:
                    upsert_alias(conn, entity_id, zh, "zh", "scw_api")
            if labels_en:
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key="foci",
                    value_text=", ".join(labels_en),
                    source="scw_api",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )

        desc_en = _localized(v.get("description") or v.get("game_description"), "en_EN")
        desc_zh = _localized(v.get("description") or v.get("game_description"), "zh_CN")
        if desc_en:
            upsert_fact(
                conn,
                entity_id=entity_id,
                key="description",
                value_text=desc_en[:2000],
                source="scw_api",
                game_version=game_version,
                ingest_run_id=run_id,
            )
        if desc_zh:
            upsert_fact(
                conn,
                entity_id=entity_id,
                key="description_zh",
                value_text=desc_zh[:2000],
                source="scw_api",
                game_version=game_version,
                ingest_run_id=run_id,
            )

        for mass_key in ("mass", "mass_total"):
            if v.get(mass_key) is not None:
                try:
                    upsert_fact(
                        conn,
                        entity_id=entity_id,
                        key=mass_key,
                        value_num=float(v[mass_key]),
                        unit="kg",
                        source="scw_api",
                        game_version=game_version,
                        ingest_run_id=run_id,
                    )
                except (TypeError, ValueError):
                    pass
        count += 1
    return count


def sync_scw(
    conn: sqlite3.Connection,
    *,
    client: ScwClient | None = None,
    vehicles: list[dict[str, Any]] | None = None,
) -> int:
    owned = client is None
    client = client or ScwClient()
    try:
        return client.sync(conn, vehicles=vehicles)
    finally:
        if owned:
            client.close()
