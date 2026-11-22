# SC Search Phase A (UEX + MCP) Implementation Plan

> **For agentic workers:** Cursor implements; Antigravity `gemini-3.8-flash` tests/verifies and git commits. Steps use checkbox (`- [ ]`) syntax for tracking. Spec: `docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md`.

**Goal:** Ship a local SQLite knowledge base with UEX price ingest and an MCP server (`sc-search`) so project agents can resolve names and query commodity prices.

**Architecture:** Installable Python package `sc_search`: adapters fetch → normalize upserts into unified tables → `query` module reads SQLite → MCP stdio tools wrap query functions. Ingest stays CLI-only (no write tools on MCP).

**Tech Stack:** Python 3.11+, `httpx`, `mcp`, `pytest`, SQLite FTS5 (tables reserved; FTS unused until Phase C).

## Global Constraints

- Package name: `sc_search`; DB default `data/sc_knowledge.sqlite`; override via `SC_SEARCH_DB`.
- User-Agent: `SC_Search-ingest/0.1`.
- UEX auth: Bearer from `UEX_API_TOKEN` in `config/.env` (never commit secrets).
- MCP tools Phase A: `sc_resolve_name`, `sc_best_prices`, `sc_terminal_prices`, `sc_data_freshness` only.
- MCP must not expose ingest/sync.
- Prices: latest snapshot only per `(commodity, terminal, side, source, game_version)`.
- Workflow: Cursor codes; delegate verify+commit to `antigravity` / `gemini-3.8-flash`.
- All Star Citizen context; no unrelated crawling.

## File map

| Path | Responsibility |
| :--- | :--- |
| `pyproject.toml` | Package metadata, deps, pytest entry |
| `.gitignore` | `.env`, `data/*.sqlite`, `data/raw/`, venv |
| `config/.env.example` | Token placeholders |
| `src/sc_search/__init__.py` | Version |
| `src/sc_search/paths.py` | Resolve project root + DB path |
| `src/sc_search/ingest/schema.sql` | DDL |
| `src/sc_search/ingest/db.py` | Connect, init schema, transactions |
| `src/sc_search/ingest/normalize.py` | Upsert entity/alias/price/run |
| `src/sc_search/ingest/adapters/uex.py` | UEX HTTP + parse |
| `src/sc_search/ingest/cli.py` | `init` / `sync uex` / `status` |
| `src/sc_search/ingest/__main__.py` | `python -m sc_search.ingest` |
| `src/sc_search/query/resolve.py` | Name → entities |
| `src/sc_search/query/prices.py` | best/terminal prices |
| `src/sc_search/query/freshness.py` | ingest_runs summary |
| `src/sc_search/mcp_server/server.py` | FastMCP tools |
| `src/sc_search/mcp_server/__main__.py` | Entry |
| `.cursor/mcp.json` | Local MCP registration |
| `AGENTS.md` | Prefer `sc-search` MCP for facts |
| `tests/` | pytest |

---

### Task 1: Repo skeleton + schema + init

**Files:**
- Create: `pyproject.toml`, `.gitignore`, `config/.env.example`
- Create: `src/sc_search/__init__.py`, `paths.py`
- Create: `src/sc_search/ingest/__init__.py`, `schema.sql`, `db.py`, `cli.py`, `__main__.py`
- Test: `tests/test_db_init.py`

**Interfaces:**
- Produces: `get_db_path() -> Path`, `connect(db_path: Path | None = None) -> sqlite3.Connection`, `init_db(conn) -> None`, CLI `init` / `status`

- [ ] **Step 1: Write failing test**

```python
# tests/test_db_init.py
from pathlib import Path
from sc_search.ingest.db import connect, init_db

def test_init_creates_core_tables(tmp_path: Path):
    db = tmp_path / "t.sqlite"
    conn = connect(db)
    init_db(conn)
    names = {r[0] for r in conn.execute(
        "SELECT name FROM sqlite_master WHERE type='table'"
    )}
    assert {"ingest_runs", "entities", "aliases", "facts", "prices", "relations", "documents"} <= names
```

- [ ] **Step 2: Run test — expect FAIL (import/module missing)**

Run: `python -m pytest tests/test_db_init.py -v`  
Expected: FAIL (no package)

- [ ] **Step 3: Implement skeleton**

`pyproject.toml`:

```toml
[project]
name = "sc-search"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["httpx>=0.27", "mcp>=1.1", "python-dotenv>=1.0"]

[project.optional-dependencies]
dev = ["pytest>=8.0"]

[build-system]
requires = ["setuptools>=68"]
build-backend = "setuptools.build_meta"

[tool.setuptools.packages.find]
where = ["src"]

[tool.setuptools.package-data]
sc_search = ["ingest/schema.sql"]

[tool.pytest.ini_options]
pythonpath = ["src"]
testpaths = ["tests"]
```

`.gitignore`: `.env`, `venv/`, `.venv/`, `__pycache__/`, `data/*.sqlite`, `data/raw/`, `*.egg-info/`, `dist/`, `build/`

`config/.env.example`:

```text
UEX_API_TOKEN=
SC_SEARCH_DB=
```

`schema.sql` — create all tables from spec §4.1 plus indexes on `aliases(alias)`, `prices(commodity_entity_id, side)`, `prices(terminal_entity_id)`, unique index for latest price key.

`db.py` — load schema SQL from package; `connect` sets `row_factory=sqlite3.Row`.

`cli.py` — argparse: `init`, `status` (status may return empty until Task 2).

- [ ] **Step 4: `pip install -e ".[dev]"` and pytest PASS**

- [ ] **Step 5: Delegate verify+commit** to antigravity `gemini-3.8-flash`  
  Brief: run pytest Task1; `git init` if needed; commit skeleton with message focusing on why (local SC knowledge schema foundation).

---

### Task 2: Normalize + UEX adapter + sync

**Files:**
- Create: `src/sc_search/ingest/normalize.py`, `adapters/__init__.py`, `adapters/uex.py`
- Modify: `cli.py` — add `sync uex`
- Test: `tests/test_normalize.py`, `tests/test_uex_adapter.py` (httpx mock)

**Interfaces:**
- Consumes: `connect`, `init_db`
- Produces:
  - `upsert_entity(conn, *, kind, canonical_name, slug=None, uuid=None, game_version) -> int`
  - `upsert_alias(conn, entity_id, alias, lang, source) -> None`
  - `replace_price(conn, *, commodity_id, terminal_id, side, price, scu_available, reported_at, source, game_version, ingest_run_id) -> None`
  - `start_run` / `finish_run`
  - `UexClient.fetch_commodities_prices_all()` / `fetch_terminals()` / `sync(conn) -> int` row count

- [ ] **Step 1: Failing tests for normalize upsert + price replace**

```python
def test_upsert_entity_is_idempotent(tmp_path):
    # init db, upsert twice same kind/name/version -> same id

def test_replace_price_keeps_single_row(tmp_path):
    # two replace_price same key -> one row, latest price
```

- [ ] **Step 2: pytest FAIL**

- [ ] **Step 3: Implement normalize**

- [ ] **Step 4: Failing test for UEX parse with fixture JSON** (no live network)

```python
# minimal terminal + price dicts shaped like UEX API
def test_uex_sync_from_fixture(tmp_path, monkeypatch):
    # monkeypatch client methods to return fixtures
    # assert entities + prices rows > 0
```

- [ ] **Step 5: Implement `adapters/uex.py`**

Base URL try `https://api.uexcorp.space/2.0` (fallback documented). Headers: `Authorization: Bearer {token}`, `User-Agent: SC_Search-ingest/0.1`. Endpoints: `terminals`, `commodities`, `commodities_prices_all` (adjust to live docs if names differ — verify against https://uexcorp.space/api/documentation/). Missing token → raise clear error, no writes.

- [ ] **Step 6: Wire `sync uex` in CLI; whole sync one transaction**

- [ ] **Step 7: pytest PASS (mocked)**

- [ ] **Step 8: Delegate verify+commit** (antigravity gemini-3.8-flash)

---

### Task 3: Query layer

**Files:**
- Create: `src/sc_search/query/__init__.py`, `resolve.py`, `prices.py`, `freshness.py`
- Test: `tests/test_query.py`

**Interfaces:**
- Produces:
  - `resolve_name(conn, text: str) -> list[dict]`
  - `best_prices(conn, commodity: str, side: str, system: str | None = None, limit: int = 10) -> list[dict]`
  - `terminal_prices(conn, terminal: str, side: str | None = None) -> list[dict]`
  - `data_freshness(conn, source: str | None = None) -> list[dict]`

- [ ] **Step 1: Seed helper in tests + failing query tests**

```python
def test_resolve_alias(tmp_path):
    # entity Gold + alias 黄金 -> resolve_name("黄金") hits Gold

def test_best_prices_sell_orders_desc(tmp_path):
    # two terminals different sell prices -> highest first
```

- [ ] **Step 2: Implement query functions** (resolve via aliases then canonical_name LIKE)

- [ ] **Step 3: pytest PASS**

- [ ] **Step 4: Delegate verify+commit**

---

### Task 4: MCP server + project wiring

**Files:**
- Create: `src/sc_search/mcp_server/__init__.py`, `server.py`, `__main__.py`
- Create: `.cursor/mcp.json`
- Modify: `AGENTS.md` — MCP-first for factual SC queries
- Test: `tests/test_mcp_tools.py` (call tool handlers directly, not full stdio)

**Interfaces:**
- Tools return JSON string: `{"ok": true|false, "data": ..., "meta": {"game_version": ..., "synced_at": ...}, "error": ...}`

- [ ] **Step 1: Failing tests invoking handler functions with temp DB**

- [ ] **Step 2: Implement FastMCP (or `mcp.server`) tools wrapping query**

Tool names: `sc_resolve_name`, `sc_best_prices`, `sc_terminal_prices`, `sc_data_freshness`.

- [ ] **Step 3: `.cursor/mcp.json`**

```json
{
  "mcpServers": {
    "sc-search": {
      "command": "python",
      "args": ["-m", "sc_search.mcp_server"],
      "cwd": "/绝对路径/SC_Search",
      "env": {
        "SC_SEARCH_DB": "/绝对路径/SC_Search/data/sc_knowledge.sqlite"
      }
    }
  }
}
```

- [ ] **Step 4: AGENTS.md short section — prefer sc-search MCP; browser for Erkul/mining/Spectrum**

- [ ] **Step 5: pytest PASS; smoke `python -m sc_search.mcp_server` starts (or import check)**

- [ ] **Step 6: Delegate final Phase A verify+commit** — run full pytest; confirm no secrets staged; commit.

---

### Task 5 (optional live): Real UEX sync

Only if user provides `UEX_API_TOKEN` via `request_secret` or local `.env`:

- [ ] `python -m sc_search.ingest init`
- [ ] `python -m sc_search.ingest sync uex`
- [ ] `python -m sc_search.ingest status`
- [ ] Manual: ask agent to call MCP tools on Gold sell prices

No commit of sqlite data file.

---

## Phase B/C (later plans)

- B: SCMDB dump → facts/relations + `sc_entity_facts`
- C: api.star-citizen.wiki + CitizenWiki + FTS + `sc_search_docs`

## Execution notes

- Cursor implements Tasks 1–4 inline in this project.
- After each task’s pytest green: `delegate_task` → antigravity / `gemini-3.8-flash` for re-test + git commit.
- Do not commit `.env` or `*.sqlite`.
