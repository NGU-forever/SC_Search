from __future__ import annotations

import json
from functools import lru_cache
from importlib import resources
from pathlib import Path
from typing import Any

from sc_search.paths import project_root

GUIDE_CATEGORIES = frozenset(
    {
        "howto",
        "client",
        "earn_meta",
        "pledge",
        "event",
        "trade_route",
        "ship_shop",
        "loadout",
        "mining",
    }
)


def _load_raw(path: Path | None = None) -> dict[str, list[dict[str, str]]]:
    if path is not None:
        return json.loads(path.read_text(encoding="utf-8"))
    packaged = resources.files("sc_search").joinpath("data/community_guides.json")
    if packaged.is_file():
        return json.loads(packaged.read_text(encoding="utf-8"))
    fallback = project_root() / "data" / "community_guides.json"
    return json.loads(fallback.read_text(encoding="utf-8"))


@lru_cache(maxsize=1)
def load_guides() -> dict[str, list[dict[str, str]]]:
    raw = _load_raw()
    out: dict[str, list[dict[str, str]]] = {}
    for key, rows in (raw or {}).items():
        if not isinstance(rows, list):
            continue
        cleaned: list[dict[str, str]] = []
        for row in rows:
            if not isinstance(row, dict):
                continue
            title = str(row.get("title") or "").strip()
            url = str(row.get("url") or "").strip()
            kind = str(row.get("kind") or "link").strip()
            if title and url:
                cleaned.append({"title": title, "url": url, "kind": kind})
        if cleaned:
            out[str(key)] = cleaned
    return out


def guides_for(category: str, *, limit: int = 4) -> list[dict[str, str]]:
    cat = (category or "").strip()
    rows = load_guides().get(cat) or []
    return list(rows[: max(0, limit)])


def browser_guide(category: str, *, limit: int = 4) -> dict[str, Any]:
    resources_list = guides_for(category, limit=limit)
    start = resources_list[0]["url"] if resources_list else ""
    return {
        "needed": True,
        "site": "guide",
        "start_url": start,
        "search_term": "",
        "resources": resources_list,
        "max_actions": 6,
        "extract": ["page title", "key steps if visible"],
        "do_not": ["open /auth", "/settings", "login", "wander other sites first"],
        "wait_confirm": True,
    }
