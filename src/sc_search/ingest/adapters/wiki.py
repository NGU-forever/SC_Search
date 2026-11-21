from __future__ import annotations

import sqlite3
import time
from typing import Any

import httpx

from sc_search.ingest.normalize import (
    clear_source_documents,
    finish_run,
    insert_document,
    start_run,
)

USER_AGENT = "SC_Search-ingest/0.1"
API = "https://starcitizen.tools/api.php"

_MANUFACTURERS = {
    "Aegis",
    "Anvil",
    "Origin",
    "Drake",
    "MISC",
    "RSI",
    "Crusader",
    "Argo",
    "Banu",
    "Esperia",
    "Mirai",
    "Kruger",
    "Greycat",
    "Tumbril",
    "Gatac",
    "Musashi",
    "Consolidated",
}


class WikiError(RuntimeError):
    pass


def wiki_title_candidates(canonical_name: str) -> list[str]:
    """Build likely starcitizen.tools titles from a ship canonical/game name."""
    name = canonical_name.strip()
    out = [name]
    parts = name.split()
    if parts and parts[0] in _MANUFACTURERS and len(parts) >= 2:
        out.append(" ".join(parts[1:]))
    # de-dupe preserve order
    seen: set[str] = set()
    uniq: list[str] = []
    for t in out:
        if t not in seen:
            seen.add(t)
            uniq.append(t)
    return uniq


class WikiClient:
    def __init__(self, client: httpx.Client | None = None, delay_s: float = 0.1) -> None:
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

    def fetch_extracts(self, titles: list[str]) -> list[dict[str, Any]]:
        out: list[dict[str, Any]] = []
        for i in range(0, len(titles), 20):
            batch = titles[i : i + 20]
            resp = self._client.get(
                API,
                params={
                    "action": "query",
                    "format": "json",
                    "prop": "extracts|info",
                    "exintro": 1,
                    "explaintext": 1,
                    "inprop": "url",
                    "redirects": 1,
                    "titles": "|".join(batch),
                },
            )
            if resp.status_code >= 400:
                raise WikiError(f"wiki HTTP {resp.status_code}")
            pages = (resp.json().get("query") or {}).get("pages") or {}
            for page in pages.values():
                if "missing" in page:
                    continue
                extract = (page.get("extract") or "").strip()
                if not extract:
                    continue
                out.append(
                    {
                        "title": page.get("title"),
                        "body": extract,
                        "url": page.get("fullurl")
                        or f"https://starcitizen.tools/{str(page.get('title','')).replace(' ', '_')}",
                        "pageid": page.get("pageid"),
                    }
                )
            if self.delay_s:
                time.sleep(self.delay_s)
        return out

    def sync_for_ships(self, conn: sqlite3.Connection, ship_names: list[str] | None = None) -> int:
        run_id = start_run(conn, "starcitizen_tools")
        game_version = "wiki-live"
        try:
            if ship_names is None:
                rows = conn.execute(
                    """
                    SELECT id, canonical_name FROM entities
                    WHERE kind = 'ship'
                    ORDER BY canonical_name
                    """
                ).fetchall()
            else:
                rows = [{"id": None, "canonical_name": n} for n in ship_names]

            titles: list[str] = []
            name_to_entity: dict[str, int | None] = {}
            for row in rows:
                entity_id = int(row["id"]) if row["id"] is not None else None
                for cand in wiki_title_candidates(str(row["canonical_name"])):
                    titles.append(cand)
                    name_to_entity[cand] = entity_id

            uniq_titles: list[str] = []
            seen: set[str] = set()
            for t in titles:
                if t not in seen:
                    seen.add(t)
                    uniq_titles.append(t)

            docs = self.fetch_extracts(uniq_titles)
            clear_source_documents(conn, "starcitizen_tools", game_version)
            count = 0
            for doc in docs:
                title = str(doc["title"])
                entity_id = name_to_entity.get(title)
                insert_document(
                    conn,
                    title=title,
                    body=str(doc["body"]),
                    url=str(doc.get("url") or ""),
                    source="starcitizen_tools",
                    game_version=game_version,
                    ingest_run_id=run_id,
                    entity_id=entity_id,
                )
                count += 1
            finish_run(conn, run_id, status="ok", row_count=count, game_version=game_version)
            return count
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise


def sync_wiki(
    conn: sqlite3.Connection,
    *,
    client: WikiClient | None = None,
    ship_names: list[str] | None = None,
    documents: list[dict[str, Any]] | None = None,
) -> int:
    if documents is not None:
        run_id = start_run(conn, "starcitizen_tools")
        game_version = "wiki-live"
        try:
            clear_source_documents(conn, "starcitizen_tools", game_version)
            count = 0
            for doc in documents:
                insert_document(
                    conn,
                    title=str(doc["title"]),
                    body=str(doc["body"]),
                    url=doc.get("url"),
                    source="starcitizen_tools",
                    game_version=game_version,
                    ingest_run_id=run_id,
                    entity_id=doc.get("entity_id"),
                )
                count += 1
            finish_run(conn, run_id, status="ok", row_count=count, game_version=game_version)
            return count
        except Exception as exc:
            finish_run(conn, run_id, status="failed", notes=str(exc))
            raise

    owned = client is None
    client = client or WikiClient()
    try:
        return client.sync_for_ships(conn, ship_names=ship_names)
    finally:
        if owned:
            client.close()
