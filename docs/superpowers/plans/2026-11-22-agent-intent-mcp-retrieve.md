# Agent Intent + MCP Retrieve Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire Agent-layer LLM understanding (AGENTS contract) to deterministic MCP retrieval—including `红枫→红风` aliases and `sc_list_missions`—so oral queries like Red Wind Junior rep grind resolve to evidence then LLM prose.

**Architecture:** AGENTS defines understand → retrieve → synthesize. Seed aliases fix 红枫. New `list_missions` query + MCP tool filters SCMDB missions by faction/rank keyword. `sc_answer` remains the simple-question fast path.

**Tech Stack:** Python 3, SQLite, pytest, MCP, `seed_aliases.json`, AGENTS.md.

**Spec:** `docs/superpowers/specs/2026-11-22-agent-intent-mcp-retrieve-design.md`

## Global Constraints

- No LLM inside MCP process; no new API keys in MCP.
- Do not invent reputation-point rates; missing → say so.
- Live version authority remains SCMDB (`live_game_version` / 4.10.3).
- Browser gates unchanged (soft verify / loadout only when allowed).
- Commits only when user asked to execute with commits / explicitly commit.

---

## File map

| File | Role |
| :--- | :--- |
| `AGENTS.md` | Understand → retrieve → synthesize + intent table |
| `data/seed_aliases.json` + `src/sc_search/data/seed_aliases.json` | 红枫物流 alias |
| `README.md` | One-line architecture |
| `src/sc_search/query/missions.py` | `list_missions(...)` |
| `src/sc_search/mcp_server/server.py` | `sc_list_missions` |
| `tests/test_missions.py` / `tests/test_mcp_tools.py` | Tests |

---

### Task 1: AGENTS understand → retrieve → synthesize

**Files:**
- Modify: `AGENTS.md`

**Interfaces:**
- Produces: Agent contract text only

- [ ] **Step 1: Insert new subsection under §1 MCP（优先）**

Add (after Live 版本 bullet, before or replacing overlapping「默认入口」as clarified below):

```markdown
* **理解 → 检索 → 成文（Agent 层）**：
  1. **理解**：先判断主意图与槽位（勿先开浏览器/WebSearch）。意图含：`mission_rep`（任务/声望）、`mission_money`、`trade`、`ship_stats`/`ship_buy`、`blueprint`/`crafting`、`loadout`、`mining`、`bug_tip`、`lore_docs`、`patch_live`、`other`。纠正常见俗称（如 **红枫物流→红风物流 / Red Wind Linehaul**）。
  2. **检索**：只调 `sc-search` 白名单工具取证据；简单事实/物价可用 `sc_answer` 快路径；声望/任务列表用 `sc_resolve_name` + `sc_list_missions`（若可用）+ `sc_entity_facts`。
  3. **成文**：只依据 MCP 证据组织中文；库无声望点数/效率则明说，不编造。
* **默认入口（简化）**：单实体静态参数/物价 → `sc_answer`；口语复杂（声望、多约束任务、对比）→ 先理解槽位再按上表检索。
```

Keep existing browser / confidence / Live bullets. Update 工具 list to include `sc_list_missions` when Task 3 lands—or add it in Task 3 Step for AGENTS tool list.

- [ ] **Step 2: No code test** — self-check AGENTS still consistent (no「中文先 resolve」冲突).

- [ ] **Step 3: Commit (if user asked)**

```bash
git add AGENTS.md
git commit -m "$(cat <<'EOF'
docs: Agent understand-retrieve-synthesize contract for SC MCP

EOF
)"
```

---

### Task 2: Alias 红枫物流 + resolve test

**Files:**
- Modify: `data/seed_aliases.json`
- Modify: `src/sc_search/data/seed_aliases.json` (keep in sync)
- Create or modify: `tests/test_aliases_seed.py` or extend `tests/test_phase_c.py` / `tests/test_query.py`

**Interfaces:**
- Consumes: `apply_seed_aliases` / `resolve_name`
- Produces: alias row linking 红枫物流 → entity Red Wind Linehaul (or canonical via seed shape)

- [ ] **Step 1: Inspect seed format**

Read one existing entry in `data/seed_aliases.json` and match shape exactly (typically `canonical` / `alias` / `lang` fields—use whatever the file uses).

- [ ] **Step 2: Add failing resolve test**

```python
def test_resolve_hongfeng_to_red_wind(tmp_path, monkeypatch):
    # init db, upsert faction Red Wind Linehaul, apply seed including 红枫物流
    # OR sync aliases from seed file after ensuring faction entity exists
    matches = resolve_name(conn, "红枫物流")
    assert any(m["canonical_name"] == "Red Wind Linehaul" for m in matches)
```

Seed application requires the entity to exist first—in test, `upsert_entity(kind="faction", canonical_name="Red Wind Linehaul")` then `apply_seed_aliases` with the new entry.

- [ ] **Step 3: Add seed entries to both JSON files**

Seed schema (match existing file):

```json
{"alias": "红枫物流", "target": "Red Wind Linehaul", "lang": "slang", "kind": "faction"},
{"alias": "红枫", "target": "Red Wind Linehaul", "lang": "slang", "kind": "faction"}
```

Add the same two objects to both `data/seed_aliases.json` and `src/sc_search/data/seed_aliases.json`.

- [ ] **Step 4: Run test — pass**

Run: `.venv/bin/python -m pytest tests/test_query.py tests/test_phase_c.py -k hongfeng -v`  
(or the file you created)

- [ ] **Step 5: Commit (if user asked)**

```bash
git add data/seed_aliases.json src/sc_search/data/seed_aliases.json tests/
git commit -m "$(cat <<'EOF'
fix: alias 红枫物流 to Red Wind Linehaul

EOF
)"
```

---

### Task 3: `list_missions` + MCP `sc_list_missions`

**Files:**
- Create: `src/sc_search/query/missions.py`
- Create: `tests/test_missions.py`
- Modify: `src/sc_search/mcp_server/server.py`
- Modify: `tests/test_mcp_tools.py`
- Modify: `AGENTS.md` / `README.md` tool lists

**Interfaces:**
- Consumes: `resolve_name`, facts `contractor` / `reward_uec` / `systems` / `mission_type`
- Produces:
  - `list_missions(conn, faction: str | None = None, rank_keyword: str | None = None, system: str | None = None, limit: int = 10) -> list[dict]`
  - MCP `sc_list_missions(faction: str | None = None, rank_keyword: str | None = None, system: str | None = None, limit: int = 10) -> str`

Each row: `canonical_name`, `title` (best alias), `contractor`, `reward_uec`, `systems`, `mission_type`, `game_version`.

- [ ] **Step 1: Failing tests**

```python
def test_list_missions_red_wind_junior(conn_with_scmdb_slice_or_seed):
    # Prefer seeding 2-3 mission entities with contractor fact + Junior alias
    rows = list_missions(conn, faction="Red Wind Linehaul", rank_keyword="Junior", limit=5)
    assert rows
    assert all("Red Wind" in (r.get("contractor") or "") for r in rows)
    assert any("Junior" in (r.get("title") or "") or "Junior" in (r.get("canonical_name") or "") for r in rows)
```

For unit test without full SCMDB dump: seed manually like other tests.

- [ ] **Step 2: Implement `missions.py`**

Logic sketch:

1. If `faction`: `resolve_name` → prefer `kind==faction` → `faction_name = canonical_name`.
2. SQL: missions where `facts.key='contractor'` and `value_text` equals/LIKE faction_name (or resolved name).
3. If `rank_keyword`: require alias or canonical_name LIKE `%rank_keyword%` COLLATE NOCASE.
4. If `system`: facts `systems` LIKE `%system%`.
5. Join reward_uec; order by reward_uec DESC NULLS LAST; LIMIT.

- [ ] **Step 3: MCP wrapper + handlers entry + AGENTS/README tool line**

- [ ] **Step 4: pytest**

Run: `.venv/bin/python -m pytest tests/test_missions.py tests/test_mcp_tools.py -v`

- [ ] **Step 5: Commit (if user asked)**

```bash
git add src/sc_search/query/missions.py src/sc_search/mcp_server/server.py tests/test_missions.py tests/test_mcp_tools.py AGENTS.md README.md
git commit -m "$(cat <<'EOF'
feat: sc_list_missions for faction/rank mission lookup

EOF
)"
```

---

### Task 4: README architecture one-liner

**Files:**
- Modify: `README.md`

- [ ] **Step 1:** In MCP / architecture section add:

```markdown
回答链路：Agent 理解意图与槽位 → `sc-search` MCP 检索证据 → Agent 依据证据成文（简单题可直接 `sc_answer`）。
```

- [ ] **Step 2:** Commit if asked.

---

## Plan self-review

| Spec § | Task |
| :--- | :--- |
| Agent 三步 + 意图表 | Task 1 |
| 红枫 alias | Task 2 |
| sc_list_missions | Task 3 |
| README | Task 4 |
| No LLM in MCP | Global |

Optional `sc_answer` mission_rep keyword hint deferred (spec 可选) — not in this plan (YAGNI).

---

## Execution handoff

Plan saved to `docs/superpowers/plans/2026-11-22-agent-intent-mcp-retrieve.md`.
