# SC Search Phase B (SCMDB) Implementation Plan

> **For agentic workers:** Cursor implements; Antigravity `gemini-3.8-flash` verifies and commits. Spec: `docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md` § stage B.

**Goal:** Ingest SCMDB merged JSON (missions/contracts, blueprint reward pools, factions) into the unified SQLite model and expose `sc_entity_facts` on MCP.

**Architecture:** Adapter downloads `versions.json` + `merged-*.json` (mirror first, then scmdb.net), maps contracts → `entities`/`facts`/`relations`, blueprint pool items → `entities`/`aliases`, then query `entity_facts` returns facts + related rewards.

**Tech Stack:** Same as Phase A (`httpx`, sqlite, pytest, MCP).

## Global Constraints

- Source id: `scmdb`. Channel: `live` (default) or `ptu`.
- Prefer mirror `https://raw.githubusercontent.com/KrovaxCode/SCMDB_DATA/main/data/` then `https://scmdb.net/data/`.
- User-Agent: `SC_Search-ingest/0.1`. Local private use of dumps.
- MCP stays read-only; CLI `sync scmdb`.
- Workflow: Cursor code; Gemini verify+commit.
- Do not commit large `data/raw/*.json` or sqlite DB.

## File map

| Path | Role |
| :--- | :--- |
| `src/sc_search/ingest/adapters/scmdb.py` | Fetch + parse + upsert |
| `src/sc_search/ingest/normalize.py` | Add `upsert_fact`, `upsert_relation` if missing |
| `src/sc_search/ingest/cli.py` | `sync scmdb [--channel]` |
| `src/sc_search/query/entities.py` | `entity_facts` |
| `src/sc_search/mcp_server/server.py` | `sc_entity_facts` |
| `tests/test_scmdb_adapter.py` | Fixture-based ingest |
| `tests/test_entity_facts.py` | Query + MCP |
| `AGENTS.md` | Mention mission/blueprint via MCP |

---

### Task 1: normalize helpers + SCMDB adapter

**Produces:** `upsert_fact`, `upsert_relation`, `ScmdbClient.sync(conn, channel=...) -> int`

Fixture JSON (tiny) with 1 faction, 1 contract (UEC + blueprintRewards), 1 blueprintPool with 2 blueprints.

- [ ] Tests → implement adapter → `python -m sc_search.ingest sync scmdb` wired
- [ ] Idempotent re-sync (facts/relations replace strategy: delete prior `source=scmdb` facts/relations for rewritten entities in that game_version, or upsert by unique keys)

**Relation types:** `mission_reward_blueprint_pool`, `pool_contains_blueprint`  
**Fact keys (mission):** `reward_uec`, `mission_type`, `category`, `illegal`, `contractor`, `systems`, `debug_name`  
**Fact keys (blueprint):** `pool_name`, `weight`, `entity_class`

### Task 2: `entity_facts` + MCP

**Produces:** `entity_facts(conn, name_or_id) -> dict` with entity, facts, relations (expanded names), aliases.

MCP tool `sc_entity_facts(name_or_id: str)`.

### Task 3: Docs + Gemini verify/commit

Update AGENTS.md one line; pytest all green; delegate antigravity gemini-3.8-flash.
