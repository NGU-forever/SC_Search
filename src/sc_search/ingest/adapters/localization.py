from __future__ import annotations

import sqlite3
from pathlib import Path

import httpx

from sc_search.ingest.normalize import finish_run, start_run, upsert_alias, upsert_entity, upsert_fact
from sc_search.paths import project_root

USER_AGENT = "SC_Search-ingest/0.1"
ORG_URL = "https://ini.42kit.com/orginal/global.ini"
FULL_URL = "https://ini.42kit.com/full/global.ini"

_NAME_PREFIXES = (
    "vehicle_Name",
    "vehicle_name",
    "items_commodities_",
    "item_Name",
    "item_name_",
    "mineabletype_",
)

_DESC_PREFIXES = ("vehicle_Desc", "vehicle_desc")


class LocalizationError(RuntimeError):
    pass


def parse_ini(text: str) -> dict[str, str]:
    text = text.lstrip("\ufeff")
    out: dict[str, str] = {}
    for line in text.splitlines():
        raw = line.strip()
        if not raw or raw.startswith(";") or raw.startswith("#") or "=" not in raw:
            continue
        key, value = raw.split("=", 1)
        out[key.strip()] = value.strip()
    return out


def _is_short_name(value: str, max_len: int = 64) -> bool:
    if not value or len(value) > max_len:
        return False
    if "\n" in value or "\\n" in value:
        return False
    if value.isdigit():
        return False
    return True


def fetch_ini_pair(
    client: httpx.Client | None = None,
    *,
    raw_dir: Path | None = None,
) -> tuple[dict[str, str], dict[str, str]]:
    owned = client is None
    client = client or httpx.Client(
        timeout=180.0,
        headers={"User-Agent": USER_AGENT},
        follow_redirects=True,
    )
    try:
        org_resp = client.get(ORG_URL)
        zh_resp = client.get(FULL_URL)
        if org_resp.status_code >= 400 or zh_resp.status_code >= 400:
            raise LocalizationError(
                f"download failed org={org_resp.status_code} zh={zh_resp.status_code}"
            )
        raw_dir = raw_dir or (project_root() / "data" / "raw")
        raw_dir.mkdir(parents=True, exist_ok=True)
        (raw_dir / "global.orginal.ini").write_bytes(org_resp.content)
        (raw_dir / "global.full.ini").write_bytes(zh_resp.content)
        return parse_ini(org_resp.text), parse_ini(zh_resp.text)
    finally:
        if owned:
            client.close()


def _build_name_index(conn: sqlite3.Connection) -> dict[str, int]:
    index: dict[str, int] = {}
    for row in conn.execute("SELECT id, canonical_name FROM entities"):
        index[str(row["canonical_name"]).casefold()] = int(row["id"])
    for row in conn.execute("SELECT entity_id, alias FROM aliases"):
        index[str(row["alias"]).casefold()] = int(row["entity_id"])
    return index


def _kind_for_key(key: str) -> str:
    lower = key.lower()
    if lower.startswith("vehicle_"):
        return "ship"
    if "commodit" in lower or lower.startswith("mineabletype_"):
        return "commodity"
    return "item"


def apply_localization_maps(
    conn: sqlite3.Connection,
    en_map: dict[str, str],
    zh_map: dict[str, str],
    *,
    run_id: int,
    game_version: str = "localization-full",
) -> int:
    index = _build_name_index(conn)
    count = 0
    keys = set(en_map) & set(zh_map)

    def remember(entity_id: int, *names: str) -> None:
        for name in names:
            if name:
                index[name.casefold()] = entity_id

    def ensure(english: str, kind: str) -> int:
        hit = index.get(english.casefold())
        if hit is not None:
            return hit
        entity_id = upsert_entity(
            conn,
            kind=kind,
            canonical_name=english,
            game_version=game_version,
            uuid=f"zh-loc-{kind}-{english}",
        )
        upsert_alias(conn, entity_id, english, "en", "localization")
        remember(entity_id, english)
        return entity_id

    # Pass 1: structured display-name keys
    for key in keys:
        if not any(key.startswith(p) for p in _NAME_PREFIXES):
            continue
        en = en_map[key]
        zh = zh_map[key]
        if not en or not zh or en == zh:
            continue
        if not _is_short_name(en) or not _is_short_name(zh, max_len=80):
            continue
        kind = _kind_for_key(key)
        entity_id = ensure(en, kind)
        upsert_alias(conn, entity_id, zh, "zh", "localization")
        remember(entity_id, zh)
        count += 1

    # Pass 1b: vehicle Chinese descriptions
    for key in keys:
        if not key.startswith(_DESC_PREFIXES):
            continue
        zh = zh_map[key]
        if not zh or len(zh) < 8:
            continue
        name_key = key.replace("vehicle_Desc", "vehicle_name").replace(
            "vehicle_desc", "vehicle_name"
        )
        ship_en = en_map.get(name_key)
        if not ship_en:
            alt = name_key.replace("vehicle_name", "vehicle_Name")
            ship_en = en_map.get(alt)
        if not ship_en:
            ship_en = en_map.get(name_key + "_short") or en_map.get(
                name_key.replace("vehicle_name", "vehicle_Name") + "_short"
            )
        if not ship_en or not _is_short_name(ship_en, max_len=80):
            continue
        entity_id = ensure(ship_en, "ship")
        upsert_fact(
            conn,
            entity_id=entity_id,
            key="description_zh",
            value_text=zh[:4000],
            source="localization",
            game_version=game_version,
            ingest_run_id=run_id,
        )
        count += 1

    # Pass 2: short EN strings that already map to known entities
    for key in keys:
        en = en_map[key]
        zh = zh_map[key]
        if not en or not zh or en == zh:
            continue
        if not _is_short_name(en) or not _is_short_name(zh, max_len=80):
            continue
        entity_id = index.get(en.casefold())
        if entity_id is None:
            continue
        upsert_alias(conn, entity_id, zh, "zh", "localization")
        remember(entity_id, zh)
        count += 1

    return count


def sync_localization(
    conn: sqlite3.Connection,
    *,
    en_map: dict[str, str] | None = None,
    zh_map: dict[str, str] | None = None,
    client: httpx.Client | None = None,
) -> int:
    run_id = start_run(conn, "localization")
    try:
        if en_map is None or zh_map is None:
            en_map, zh_map = fetch_ini_pair(client)
        rows = apply_localization_maps(conn, en_map, zh_map, run_id=run_id)
        finish_run(
            conn,
            run_id,
            status="ok",
            row_count=rows,
            game_version="localization-full",
            notes="汉化组 global.ini CC BY-NC-SA 4.0; personal non-commercial use",
        )
        return rows
    except Exception as exc:
        finish_run(conn, run_id, status="failed", notes=str(exc))
        raise
