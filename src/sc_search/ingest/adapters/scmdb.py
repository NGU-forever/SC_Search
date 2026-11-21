from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any

import httpx

from sc_search.ingest.normalize import (
    clear_source_facts_relations,
    finish_run,
    insert_relation,
    start_run,
    upsert_alias,
    upsert_entity,
    upsert_fact,
)
from sc_search.paths import project_root

USER_AGENT = "SC_Search-ingest/0.1"
MIRROR_BASE = "https://raw.githubusercontent.com/KrovaxCode/SCMDB_DATA/main/data"
SCMDB_BASE = "https://scmdb.net/data"


class ScmdbError(RuntimeError):
    pass


class ScmdbClient:
    def __init__(
        self,
        client: httpx.Client | None = None,
        raw_dir: Path | None = None,
    ) -> None:
        self._client = client or httpx.Client(
            timeout=120.0,
            headers={"User-Agent": USER_AGENT, "Accept": "application/json"},
            follow_redirects=True,
        )
        self._owns_client = client is None
        self.raw_dir = raw_dir or (project_root() / "data" / "raw")

    def close(self) -> None:
        if self._owns_client:
            self._client.close()

    def list_versions(self) -> list[dict[str, str]]:
        body = self._get_json("versions.json")
        if not isinstance(body, list):
            raise ScmdbError("versions.json was not a list")
        out = [
            item
            for item in body
            if isinstance(item, dict) and item.get("version") and item.get("file")
        ]
        if not out:
            raise ScmdbError("no SCMDB versions available")
        return out

    def resolve_version(self, channel: str = "live", version: str | None = None) -> dict[str, str]:
        versions = self.list_versions()
        if version:
            needle = version.casefold().strip()
            for item in versions:
                ver = str(item["version"]).casefold()
                if ver == needle or needle in ver:
                    return item
            raise ScmdbError(f"SCMDB version not found: {version}")

        channel = (channel or "live").casefold().strip()
        if channel in {"latest", "any", "all"}:
            return versions[0]
        if channel not in {"live", "ptu"}:
            raise ScmdbError("channel must be live, ptu, or latest")
        for item in versions:
            if f"-{channel}." in str(item["version"]).casefold():
                return item
        return versions[0]

    def fetch_merged(self, file_name: str) -> dict[str, Any]:
        body = self._get_json(file_name)
        if not isinstance(body, dict):
            raise ScmdbError(f"{file_name} was not a JSON object")
        self.raw_dir.mkdir(parents=True, exist_ok=True)
        (self.raw_dir / file_name).write_text(json.dumps(body), encoding="utf-8")
        return body

    def _get_json(self, path: str) -> Any:
        last_error: Exception | None = None
        for base in (MIRROR_BASE, SCMDB_BASE):
            url = f"{base.rstrip('/')}/{path.lstrip('/')}"
            try:
                resp = self._client.get(url)
                if resp.status_code >= 400:
                    last_error = ScmdbError(f"HTTP {resp.status_code} for {url}")
                    continue
                text = resp.text.lstrip("\ufeff").lstrip()
                if text.startswith("<"):
                    last_error = ScmdbError(f"HTML instead of JSON at {url}")
                    continue
                return resp.json()
            except Exception as exc:  # noqa: BLE001 — try next mirror
                last_error = exc
        raise ScmdbError(f"failed to fetch {path}: {last_error}")

    def sync(
        self,
        conn: sqlite3.Connection,
        *,
        channel: str = "live",
        version: str | None = None,
        payload: dict[str, Any] | None = None,
    ) -> int:
        run_id = start_run(conn, "scmdb")
        try:
            if payload is None:
                selected = self.resolve_version(channel=channel, version=version)
                payload = self.fetch_merged(str(selected["file"]))
                game_version = str(payload.get("version") or selected["version"])
            else:
                game_version = str(payload.get("version") or version or channel)
            rows = apply_scmdb_payload(conn, payload, run_id=run_id, game_version=game_version)
            finish_run(conn, run_id, status="ok", row_count=rows, game_version=game_version)
            return rows
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise


def apply_scmdb_payload(
    conn: sqlite3.Connection,
    payload: dict[str, Any],
    *,
    run_id: int,
    game_version: str,
) -> int:
    clear_source_facts_relations(conn, "scmdb", game_version)
    count = 0

    factions = payload.get("factions") or {}
    faction_ids: dict[str, int] = {}
    if isinstance(factions, dict):
        for guid, meta in factions.items():
            if not isinstance(meta, dict):
                continue
            name = meta.get("name") or guid
            entity_id = upsert_entity(
                conn,
                kind="faction",
                canonical_name=str(name),
                game_version=game_version,
                uuid=f"scmdb-faction-{guid}",
            )
            upsert_alias(conn, entity_id, str(name), "en", "scmdb")
            faction_ids[str(guid)] = entity_id
            count += 1

    pool_ids: dict[str, int] = {}
    blueprint_pools = payload.get("blueprintPools") or {}
    if isinstance(blueprint_pools, dict):
        for pool_guid, pool in blueprint_pools.items():
            if not isinstance(pool, dict):
                continue
            pool_name = str(pool.get("name") or pool_guid)
            pool_entity = upsert_entity(
                conn,
                kind="blueprint_pool",
                canonical_name=pool_name,
                game_version=game_version,
                uuid=f"scmdb-bppool-{pool_guid}",
            )
            upsert_alias(conn, pool_entity, pool_name, "en", "scmdb")
            pool_ids[str(pool_guid)] = pool_entity
            upsert_fact(
                conn,
                entity_id=pool_entity,
                key="source_tag",
                value_text=str(pool.get("source") or ""),
                source="scmdb",
                game_version=game_version,
                ingest_run_id=run_id,
            )
            count += 1
            for bp in pool.get("blueprints") or []:
                if not isinstance(bp, dict):
                    continue
                bp_name = str(bp.get("name") or bp.get("blueprintRecord") or "unknown")
                bp_uuid = str(bp.get("blueprintRecord") or bp_name)
                bp_entity = upsert_entity(
                    conn,
                    kind="blueprint",
                    canonical_name=bp_name,
                    game_version=game_version,
                    uuid=f"scmdb-bp-{bp_uuid}",
                )
                upsert_alias(conn, bp_entity, bp_name, "en", "scmdb")
                weight = bp.get("weight")
                upsert_fact(
                    conn,
                    entity_id=bp_entity,
                    key="weight",
                    value_num=float(weight) if weight is not None else None,
                    value_text=None if weight is not None else None,
                    source="scmdb",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )
                if bp.get("entityClass"):
                    upsert_fact(
                        conn,
                        entity_id=bp_entity,
                        key="entity_class",
                        value_text=str(bp["entityClass"]),
                        source="scmdb",
                        game_version=game_version,
                        ingest_run_id=run_id,
                    )
                upsert_fact(
                    conn,
                    entity_id=bp_entity,
                    key="pool_name",
                    value_text=pool_name,
                    source="scmdb",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )
                insert_relation(
                    conn,
                    from_id=pool_entity,
                    to_id=bp_entity,
                    rel_type="pool_contains_blueprint",
                    source="scmdb",
                    game_version=game_version,
                    qty=float(weight) if weight is not None else None,
                )
                count += 1

    contracts = payload.get("contracts") or []
    if isinstance(contracts, list):
        for contract in contracts:
            if not isinstance(contract, dict):
                continue
            # Prefer debugName as canonical — many contracts share placeholder titles.
            debug_name = contract.get("debugName")
            title = str(contract.get("title") or debug_name or contract.get("id") or "mission")
            canonical = str(debug_name or title)
            mission_id = str(contract.get("id") or debug_name or title)
            entity_id = upsert_entity(
                conn,
                kind="mission",
                canonical_name=canonical,
                game_version=game_version,
                slug=str(debug_name or "") or None,
                uuid=f"scmdb-mission-{mission_id}",
            )
            upsert_alias(conn, entity_id, canonical, "en", "scmdb")
            if title != canonical:
                upsert_alias(conn, entity_id, title, "en", "scmdb")

            def fact_text(key: str, value: Any) -> None:
                if value is None or value == "":
                    return
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key=key,
                    value_text=str(value),
                    source="scmdb",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )

            def fact_num(key: str, value: Any, unit: str | None = None) -> None:
                if value is None or value == "":
                    return
                try:
                    num = float(value)
                except (TypeError, ValueError):
                    return
                upsert_fact(
                    conn,
                    entity_id=entity_id,
                    key=key,
                    value_num=num,
                    unit=unit,
                    source="scmdb",
                    game_version=game_version,
                    ingest_run_id=run_id,
                )

            fact_num("reward_uec", contract.get("rewardUEC"), "aUEC")
            fact_text("mission_type", contract.get("missionType") or contract.get("missionTypeKey"))
            fact_text("category", contract.get("category"))
            fact_text("contractor", contract.get("contractorName"))
            fact_text("debug_name", contract.get("debugName"))
            if contract.get("illegal") is not None:
                fact_text("illegal", "1" if contract.get("illegal") else "0")
            systems = contract.get("availableSystems") or contract.get("systems")
            if systems:
                fact_text("systems", ",".join(str(s) for s in systems) if isinstance(systems, list) else str(systems))

            faction_guid = contract.get("factionGuid")
            if faction_guid and str(faction_guid) in faction_ids:
                insert_relation(
                    conn,
                    from_id=entity_id,
                    to_id=faction_ids[str(faction_guid)],
                    rel_type="mission_faction",
                    source="scmdb",
                    game_version=game_version,
                )

            for reward in contract.get("blueprintRewards") or []:
                if not isinstance(reward, dict):
                    continue
                pool_guid = str(reward.get("blueprintPool") or "")
                pool_entity = pool_ids.get(pool_guid)
                if pool_entity is None and reward.get("poolName"):
                    pool_entity = upsert_entity(
                        conn,
                        kind="blueprint_pool",
                        canonical_name=str(reward["poolName"]),
                        game_version=game_version,
                        uuid=f"scmdb-bppool-{pool_guid or reward['poolName']}",
                    )
                    pool_ids[pool_guid] = pool_entity
                if pool_entity is None:
                    continue
                insert_relation(
                    conn,
                    from_id=entity_id,
                    to_id=pool_entity,
                    rel_type="mission_reward_blueprint_pool",
                    source="scmdb",
                    game_version=game_version,
                    qty=float(reward["chance"]) if reward.get("chance") is not None else None,
                    meta_json=json.dumps(
                        {"poolName": reward.get("poolName"), "trigger": reward.get("trigger")},
                        ensure_ascii=False,
                    ),
                )

            for item in contract.get("itemRewards") or []:
                if not isinstance(item, dict):
                    continue
                item_name = str(item.get("name") or item.get("entityClass") or "item")
                item_uuid = str(item.get("entityClass") or item_name)
                item_entity = upsert_entity(
                    conn,
                    kind="item",
                    canonical_name=item_name,
                    game_version=game_version,
                    uuid=f"scmdb-item-{item_uuid}",
                )
                upsert_alias(conn, item_entity, item_name, "en", "scmdb")
                insert_relation(
                    conn,
                    from_id=entity_id,
                    to_id=item_entity,
                    rel_type="mission_reward_item",
                    source="scmdb",
                    game_version=game_version,
                    qty=float(item["amount"]) if item.get("amount") is not None else None,
                )
            count += 1

    return count


def sync_scmdb(
    conn: sqlite3.Connection,
    *,
    channel: str = "live",
    version: str | None = None,
    client: ScmdbClient | None = None,
    payload: dict[str, Any] | None = None,
) -> int:
    owned = client is None and payload is None
    client = client or ScmdbClient()
    try:
        return client.sync(conn, channel=channel, version=version, payload=payload)
    finally:
        if owned:
            client.close()
