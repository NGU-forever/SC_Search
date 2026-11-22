# sc_query_plan Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `sc_query_plan` so Agents route SC questions to MCP-only vs Erkul/mining browser with a short, testable playbook—cutting wrong browser opens and long SPA clicks.

**Architecture:** Pure function `query_plan(conn, question, hint_entity=None)` in `sc_search.query.plan` classifies intent via keyword rules + `resolve_name`, returns a structured plan. MCP tool `sc_query_plan` wraps it. `AGENTS.md` mandates Agents call this first and obey `browser.needed` / `max_actions`. No Erkul ingest.

**Tech Stack:** Python 3, SQLite (existing), pytest, MCP Python SDK (`mcp.server.mcpserver`), existing `resolve_name`.

**Spec:** `docs/superpowers/specs/2026-11-21-sc-query-plan-routing-design.md`

## Global Constraints

- Do **not** crawl or store Erkul/mining-site payload data.
- Return shape matches existing tools: JSON string `{ok, data, meta, error}`.
- Erkul default URL: `https://erkul.games/calculator`; mining default: `https://regolith.rocks`.
- Forbid browser targets `/auth/` and `/settings`.
- Prefer English `canonical_name` as `browser.search_term`.
- Do not commit unless the user explicitly asks (repo user rule overrides plan “Commit” steps—skip or ask).

---

## File map

| File | Role |
| :--- | :--- |
| `src/sc_search/query/plan.py` | Intent classification + plan dict |
| `src/sc_search/mcp_server/server.py` | Register `sc_query_plan` + handlers entry |
| `tests/test_query_plan.py` | Unit tests for `query_plan` |
| `tests/test_mcp_tools.py` | Handler smoke for `sc_query_plan` |
| `AGENTS.md` | Hard routing + browser playbook |
| `README.md` | One-line tool list (optional in Task 3) |

---

### Task 1: `query_plan` core + unit tests

**Files:**
- Create: `src/sc_search/query/plan.py`
- Create: `tests/test_query_plan.py`
- Modify: none required beyond imports from existing `resolve_name` / db helpers

**Interfaces:**
- Consumes: `resolve_name(conn, text) -> list[dict]` from `sc_search.query.resolve`
- Produces: `query_plan(conn: sqlite3.Connection, question: str, hint_entity: str | None = None) -> dict` with keys `intent`, `path`, `confidence`, `entities`, `mcp_steps`, `browser`, `stop_if`, `reason`

- [ ] **Step 1: Write failing tests**

Create `tests/test_query_plan.py`:

```python
from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import upsert_alias, upsert_entity
from sc_search.query.plan import query_plan


def _seed_ship(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    eid = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, eid, "卡拉克", "zh", "manual")
    upsert_alias(conn, eid, "大白鲨", "zh", "seed")  # optional; Carrack is not 大白鲨—skip if confusing
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    conn.commit()
    conn.close()


@pytest.fixture
def conn(tmp_path: Path):
    db = tmp_path / "p.sqlite"
    _seed_ship(db)
    c = connect(db)
    yield c
    c.close()


def test_facts_cargo_mcp_only(conn):
    p = query_plan(conn, "卡拉克货舱多大")
    assert p["intent"] == "facts"
    assert p["path"] == "mcp_only"
    assert p["browser"]["needed"] is False
    assert p["entities"][0]["canonical_name"] == "Carrack"


def test_prices_mcp_only(conn):
    p = query_plan(conn, "黄金在哪卖")
    assert p["intent"] == "prices"
    assert p["path"] == "mcp_only"
    assert p["browser"]["needed"] is False


def test_loadout_zh(conn):
    p = query_plan(conn, "卡拉克怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["needed"] is True
    assert p["browser"]["start_url"] == "https://erkul.games/calculator"
    assert p["browser"]["search_term"] == "Carrack"
    assert p["browser"]["max_actions"] == 8


def test_loadout_en(conn):
    p = query_plan(conn, "Carrack best DPS loadout")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["search_term"] == "Carrack"


def test_mining_calc(conn):
    p = query_plan(conn, "鼹鼠怎么配激光头")
    assert p["intent"] == "mining_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["start_url"] == "https://regolith.rocks"
```

Remove the bogus 大白鲨 alias line from `_seed_ship` when implementing (Carrack ≠ 大白鲨). Seed only Carrack + 卡拉克 + Gold/黄金.

- [ ] **Step 2: Run tests — expect fail**

Run: `python -m pytest tests/test_query_plan.py -v`

Expected: `ImportError` or `ModuleNotFoundError` for `sc_search.query.plan`.

- [ ] **Step 3: Implement `plan.py`**

Create `src/sc_search/query/plan.py`:

```python
from __future__ import annotations

import re
import sqlite3

from sc_search.query.resolve import resolve_name

ERKUL_CALCULATOR = "https://erkul.games/calculator"
REGOLITH = "https://regolith.rocks"

_LOADOUT_RE = re.compile(
    r"(dps|loadout|erkul|配装|怎么配|如何配|打炮|火力|护盾续航)",
    re.I,
)
_MINING_RE = re.compile(
    r"(破矿|精炼模拟|regolith|激光头|采矿头|mining\s*laser)",
    re.I,
)
_PRICES_RE = re.compile(
    r"(价格|报价|哪[买卖出]|买点|卖点|\bbuy\b|\bsell\b|price)",
    re.I,
)
_LIVE_RE = re.compile(
    r"(patch\s*notes?|spectrum|补丁说明|官方论坛)",
    re.I,
)
_DOCS_RE = re.compile(
    r"(lore|设定|背景|wiki\s*摘要|是什么来头)",
    re.I,
)


def _entity_brief(m: dict) -> dict:
    brief = {
        "id": m["id"],
        "kind": m["kind"],
        "canonical_name": m["canonical_name"],
    }
    lang = m.get("matched_lang")
    alias = m.get("matched_alias")
    if lang == "zh" and alias:
        brief["zh"] = alias
    return brief


def _resolve_entities(conn: sqlite3.Connection, texts: list[str]) -> list[dict]:
    seen: set[int] = set()
    out: list[dict] = []
    for t in texts:
        t = (t or "").strip()
        if not t:
            continue
        for m in resolve_name(conn, t):
            eid = int(m["id"])
            if eid in seen:
                continue
            seen.add(eid)
            out.append(_entity_brief(m))
    return out


def _browser_off() -> dict:
    return {"needed": False}


def _browser_erkul(search_term: str) -> dict:
    return {
        "needed": True,
        "site": "erkul",
        "start_url": ERKUL_CALCULATOR,
        "search_term": search_term,
        "max_actions": 8,
        "extract": ["selected ship", "DPS summary", "shield / power notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
    }


def _browser_mining(search_term: str) -> dict:
    return {
        "needed": True,
        "site": "regolith",
        "start_url": REGOLITH,
        "search_term": search_term,
        "max_actions": 8,
        "extract": ["laser / module setup", "instability / resistance notes"],
        "do_not": ["open /auth", "/settings", "wander other sites first"],
    }


def query_plan(
    conn: sqlite3.Connection,
    question: str,
    hint_entity: str | None = None,
) -> dict:
    q = (question or "").strip()
    hint = (hint_entity or "").strip() or None

    resolve_texts = [t for t in [hint, q] if t]
    entities = _resolve_entities(conn, resolve_texts)
    primary = entities[0]["canonical_name"] if entities else (hint or "")

    if _LOADOUT_RE.search(q):
        steps = []
        if q:
            steps.append({"tool": "sc_resolve_name", "args": {"text": hint or q}})
        if primary:
            steps.append({"tool": "sc_entity_facts", "args": {"name_or_id": primary}})
        return {
            "intent": "loadout_calc",
            "path": "mcp_then_browser",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": steps,
            "browser": _browser_erkul(primary or q),
            "stop_if": "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
            "reason": "配装/DPS 需 Erkul；库无实时计算",
        }

    if _MINING_RE.search(q):
        return {
            "intent": "mining_calc",
            "path": "mcp_then_browser",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": (
                [{"tool": "sc_resolve_name", "args": {"text": hint or q}}] if q else []
            ),
            "browser": _browser_mining(primary or q),
            "stop_if": "只要矿船静态参数时不开采矿站",
            "reason": "采矿配装/模拟需交互站",
        }

    if _PRICES_RE.search(q):
        commodity = primary or q
        return {
            "intent": "prices",
            "path": "mcp_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_resolve_name", "args": {"text": hint or q}},
                {"tool": "sc_best_prices", "args": {"commodity": commodity, "side": "sell"}},
            ],
            "browser": _browser_off(),
            "stop_if": "有报价则不要开浏览器",
            "reason": "物价走本地 UEX 快照",
        }

    if _LIVE_RE.search(q):
        return {
            "intent": "live_web",
            "path": "browser_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [],
            "browser": {
                "needed": True,
                "site": "rsi",
                "start_url": "https://robertsspaceindustries.com",
                "search_term": q,
                "max_actions": 8,
                "extract": ["patch version", "relevant notes"],
                "do_not": ["open /auth", "/settings"],
            },
            "stop_if": "",
            "reason": "Patch/Spectrum 需官网或论坛",
        }

    if _DOCS_RE.search(q):
        return {
            "intent": "docs",
            "path": "mcp_only",
            "confidence": "medium",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_search_docs", "args": {"query": q, "limit": 5}},
            ],
            "browser": _browser_off(),
            "stop_if": "库有摘要则不要开浏览器",
            "reason": "本地 Wiki FTS",
        }

    if entities:
        name = entities[0]["canonical_name"]
        return {
            "intent": "facts",
            "path": "mcp_only",
            "confidence": "high",
            "entities": entities,
            "mcp_steps": [
                {"tool": "sc_resolve_name", "args": {"text": hint or q}},
                {"tool": "sc_entity_facts", "args": {"name_or_id": name}},
            ],
            "browser": _browser_off(),
            "stop_if": "facts 已够答则不要开浏览器",
            "reason": "静态实体事实走本地库",
        }

    return {
        "intent": "facts",
        "path": "mcp_only",
        "confidence": "low",
        "entities": [],
        "mcp_steps": [{"tool": "sc_resolve_name", "args": {"text": q}}],
        "browser": _browser_off(),
        "stop_if": "未解析到实体前不要盲目开浏览器",
        "reason": "无法判断意图；先 resolve",
    }
```

Fix seed in tests: only Carrack/卡拉克 and Gold/黄金 (no wrong 大白鲨 alias).

- [ ] **Step 4: Run tests — expect pass**

Run: `python -m pytest tests/test_query_plan.py -v`

Expected: all PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add src/sc_search/query/plan.py tests/test_query_plan.py
git commit -m "$(cat <<'EOF'
feat: add query_plan for MCP vs browser routing

EOF
)"
```

---

### Task 2: MCP `sc_query_plan` tool

**Files:**
- Modify: `src/sc_search/mcp_server/server.py`
- Modify: `tests/test_mcp_tools.py`

**Interfaces:**
- Consumes: `query_plan(conn, question, hint_entity=None) -> dict`
- Produces: MCP tool `sc_query_plan(question: str, hint_entity: str | None = None) -> str` JSON; `handlers["sc_query_plan"]`

- [ ] **Step 1: Extend MCP test**

In `tests/test_mcp_tools.py`, extend `_seed` to add a ship + alias, and assert:

```python
    ship = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, ship, "卡拉克", "zh", "manual")
    # ... existing gold seed ...

    plan = json.loads(mcp_server.handlers["sc_query_plan"]("卡拉克怎么配打炮"))
    assert plan["ok"] is True
    assert plan["data"]["intent"] == "loadout_calc"
    assert plan["data"]["browser"]["search_term"] == "Carrack"
```

- [ ] **Step 2: Run test — expect fail**

Run: `python -m pytest tests/test_mcp_tools.py::test_mcp_handlers -v`

Expected: KeyError `sc_query_plan` or assert fail.

- [ ] **Step 3: Register tool in `server.py`**

Add import and tool:

```python
from sc_search.query.plan import query_plan

@mcp.tool()
def sc_query_plan(question: str, hint_entity: str | None = None) -> str:
    """Plan MCP vs browser steps for a Star Citizen question (no Erkul scrape)."""
    try:
        conn = open_db()
        try:
            data = query_plan(conn, question, hint_entity=hint_entity)
            return _ok(data)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))
```

Add to `handlers` dict: `"sc_query_plan": sc_query_plan`.

- [ ] **Step 4: Run tests — expect pass**

Run: `python -m pytest tests/test_mcp_tools.py tests/test_query_plan.py -v`

Expected: all PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add src/sc_search/mcp_server/server.py tests/test_mcp_tools.py
git commit -m "$(cat <<'EOF'
feat: expose sc_query_plan MCP tool

EOF
)"
```

---

### Task 3: AGENTS.md (+ optional README)

**Files:**
- Modify: `AGENTS.md` (MCP bullet + section 6)
- Modify: `README.md` (tool list line, if present)

**Interfaces:**
- Consumes: tool name `sc_query_plan` and fields from Task 1
- Produces: Agent-facing rules only (no new code)

- [ ] **Step 1: Update `AGENTS.md` MCP bullets**

Replace/extend the「本地知识库 MCP」bullets so they include:

```markdown
* **本地知识库 MCP（优先）**：
  * 涉及物价、终端、货物/飞船名称、任务/蓝图、Wiki 摘要、数据新鲜度、以及「要不要开浏览器」等，优先调用本项目 MCP **`sc-search`**。
  * **强制顺序**：先 `sc_query_plan(question)` → 按返回的 `mcp_steps` 调用 → **仅当** `browser.needed=true` 才开浏览器。禁止先 WebSearch / 先开 Erkul 再查库。
  * `path=mcp_only`：答完即停；禁止为「核对」再开 Erkul。缺字段如实说明。
  * 工具：`sc_query_plan`、`sc_resolve_name`、`sc_best_prices`、`sc_terminal_prices`、`sc_data_freshness`、`sc_entity_facts`、`sc_search_docs`。
  * 写入/同步（MCP 只读）：`sync uex`｜`sync scmdb [--channel live|ptu]`｜`sync scw`｜`sync wiki`｜`sync aliases`｜`sync zh`。
  * 中文提问：plan / resolve 用原话；浏览器 `search_term` 必须用 plan 给出的英文标准名。
```

- [ ] **Step 2: Rewrite section 6 browser protocol**

```markdown
## 6. 浏览器交互与自动化规范

* **仅在 `sc_query_plan` 要求时开浏览器**（`browser.needed=true`）。日常事实查询不以浏览器为优先。
* **剧本**：只用 plan 的 `start_url`；`search_term` 为英文标准名；动作 ≤ `max_actions`（默认 8）：打开 → 搜索/定位 → 提取 `extract` 列表 → **关闭标签**。
* **禁止**：`/auth`、`/settings`；同一题未在 plan 中写明时另开第二站；用未解析的中文俗称当 Erkul 搜索词。
* **失败降级**：超时或失败时，用已有 MCP facts 答静态部分，并标明「配装/DPS 未取到」。
* 需要浏览器自动化时，优先 `ego-browser`（ego lite），但仍须服从上述 plan。
```

- [ ] **Step 3: Optional README one-liner**

If README lists MCP tools, append `` `sc_query_plan` `` with short description「意图路由：MCP vs 浏览器」。

- [ ] **Step 4: Sanity — no code test; re-run unit tests**

Run: `python -m pytest tests/test_query_plan.py tests/test_mcp_tools.py -v`

Expected: PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add AGENTS.md README.md
git commit -m "$(cat <<'EOF'
docs: require sc_query_plan before browser for SC queries

EOF
)"
```

---

## Plan self-review

| Spec section | Task |
| :--- | :--- |
| Intent table + paths | Task 1 |
| `sc_query_plan` API / browser object | Task 1–2 |
| AGENTS hard rules + playbook | Task 3 |
| Unit + manual acceptance cases | Task 1 tests + AGENTS copy |
| No Erkul ingest | Global Constraints; no adapter task |

No TBD placeholders. Types consistent: `query_plan(...) -> dict`, MCP `sc_query_plan(question, hint_entity=None) -> str`.

---

## Execution handoff

Plan saved to `docs/superpowers/plans/2026-11-21-sc-query-plan-routing.md`.
