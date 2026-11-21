from __future__ import annotations

import os
from pathlib import Path


def project_root() -> Path:
    return Path(__file__).resolve().parents[2]


def get_db_path() -> Path:
    override = os.environ.get("SC_SEARCH_DB")
    if override:
        return Path(override).expanduser().resolve()
    return project_root() / "data" / "sc_knowledge.sqlite"
