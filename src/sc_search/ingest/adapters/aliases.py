from __future__ import annotations

import json
import sqlite3
from pathlib import Path
from typing import Any

import httpx

from sc_search.ingest.normalize import finish_run, start_run, upsert_alias, upsert_entity
from sc_search.paths import project_root
from sc_search.query.resolve import resolve_name

USER_AGENT = "SC_Search-ingest/0.1"
CITIZENWIKI_API = "https://citizenwiki.cn/api.php"


class AliasError(RuntimeError):
    pass


def load_seed_aliases(path: Path | None = None) -> list[dict[str, str]]:
    target = path if path is not None else project_root() / "data" / "seed_aliases.json"
    return json.loads(target.read_text(encoding="utf-8"))


def apply_seed_aliases(
    conn: sqlite3.Connection,
    entries: list[dict[str, str]],
    *,
    game_version: str = "aliases",
) -> int:
    count = 0
    for entry in entries:
        alias = (entry.get("alias") or "").strip()
        target = (entry.get("target") or "").strip()
        lang = (entry.get("lang") or "zh").strip()
        kind = (entry.get("kind") or "ship").strip()
        if not alias or not target:
            continue
        matches = resolve_name(conn, target)
        entity_id = None
        exact = [
            m
            for m in matches
            if str(m.get("canonical_name", "")).casefold() == target.casefold()
            or str(m.get("matched_alias", "")).casefold() == target.casefold()
        ]
        pool = exact or matches
        kind_hits = [m for m in pool if m.get("kind") == kind]
        if kind_hits:
            entity_id = int(kind_hits[0]["id"])
        else:
            # Do not attach slang to a wrong kind (e.g. mission containing "Quantanium").
            entity_id = upsert_entity(
                conn,
                kind=kind,
                canonical_name=target,
                game_version=game_version,
                uuid=f"alias-target-{kind}-{target}",
            )
            upsert_alias(conn, entity_id, target, "en", "seed")
        upsert_alias(conn, entity_id, alias, lang, "seed")
        count += 1
    return count


def try_citizenwiki_probe(client: httpx.Client | None = None) -> dict[str, Any]:
    """Soft probe — Cloudflare often blocks automated clients."""
    owned = client is None
    client = client or httpx.Client(
        timeout=30.0,
        headers={"User-Agent": USER_AGENT, "Accept": "application/json"},
        follow_redirects=True,
    )
    try:
        resp = client.get(
            CITIZENWIKI_API,
            params={"action": "query", "format": "json", "meta": "siteinfo"},
        )
        if resp.status_code == 200 and resp.text.lstrip().startswith("{"):
            return {"ok": True, "status": resp.status_code}
        return {
            "ok": False,
            "status": resp.status_code,
            "error": "citizenwiki blocked or non-JSON (likely Cloudflare)",
        }
    finally:
        if owned:
            client.close()


def sync_aliases(
    conn: sqlite3.Connection,
    *,
    seed_path: Path | None = None,
    entries: list[dict[str, str]] | None = None,
    probe_citizenwiki: bool = True,
) -> int:
    run_id = start_run(conn, "aliases")
    try:
        entries = entries if entries is not None else load_seed_aliases(seed_path)
        count = apply_seed_aliases(conn, entries)
        notes = None
        if probe_citizenwiki:
            probe = try_citizenwiki_probe()
            if not probe.get("ok"):
                notes = str(probe.get("error") or "citizenwiki unavailable")
        finish_run(
            conn,
            run_id,
            status="ok",
            row_count=count,
            game_version="aliases",
            notes=notes,
        )
        return count
    except Exception as exc:
        finish_run(conn, run_id, status="failed", notes=str(exc))
        raise
