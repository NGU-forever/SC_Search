# sc_answer One-Shot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `sc_answer` so one MCP call classifies the question, runs local queries, and returns a short Chinese `answer_draft` plus `evidence`; detail expands on demand; browser only when the existing plan gate says so.

**Architecture:** `answer_question(conn, question, detail_level, hint_entity)` calls `query_plan`, then dispatches to `entity_facts` / `best_prices` / `search_docs` (no LLM). Templates build `answer_draft`. MCP wraps the function. AGENTS makes `sc_answer` the default entry.

**Tech Stack:** Python 3, SQLite, pytest, existing `sc_search.query.*`, MCP Python SDK.

**Spec:** `docs/superpowers/specs/2026-11-21-sc-answer-oneshot-design.md`

## Global Constraints

- No Erkul/mining crawl or ingest; no LLM inside the tool.
- Reuse `query_plan` for intent/path/browser; do not reimplement routing regexes.
- Default `detail_level="brief"`; `full` returns thicker evidence with caps.
- Never invent numeric facts; missing → explicit 库未覆盖.
- `browser.search_term` remains English canonical only (from plan).
- Commits: only when the user asked to execute the plan with commits, or explicitly asked to commit (repo user rule).

---

## File map

| File | Role |
| :--- | :--- |
| `src/sc_search/query/answer.py` | `answer_question` + templates + evidence builders |
| `src/sc_search/mcp_server/server.py` | Register `sc_answer` |
| `tests/test_answer.py` | Unit tests |
| `tests/test_mcp_tools.py` | Handler smoke |
| `AGENTS.md` / `README.md` | Default entry = `sc_answer` |

---

### Task 1: `answer_question` + unit tests

**Files:**
- Create: `src/sc_search/query/answer.py`
- Create: `tests/test_answer.py`

**Interfaces:**
- Consumes: `query_plan(conn, question, hint_entity=None) -> dict`; `entity_facts(conn, name_or_id) -> dict | None`; `best_prices(conn, commodity, side, system=None, limit=10) -> list[dict]`; `search_docs(conn, query, limit=5) -> list[dict]`
- Produces: `answer_question(conn: sqlite3.Connection, question: str, detail_level: str = "brief", hint_entity: str | None = None) -> dict` with keys `intent`, `path`, `confidence`, `answer_draft`, `evidence`, `detail_available`, `follow_ups`, `browser`, `plan`

- [ ] **Step 1: Write failing tests**

Create `tests/test_answer.py`:

```python
from pathlib import Path

import pytest

from sc_search.ingest.db import connect, init_db
from sc_search.ingest.normalize import replace_price, start_run, upsert_alias, upsert_entity, upsert_fact
from sc_search.query.answer import answer_question


def _seed(db_path: Path) -> None:
    conn = connect(db_path)
    init_db(conn)
    run = start_run(conn, "scw", "4.10.3")
    ship = upsert_entity(conn, kind="ship", canonical_name="Carrack", game_version="4.10.3")
    upsert_alias(conn, ship, "卡拉克", "zh", "manual")
    upsert_fact(
        conn,
        entity_id=ship,
        key="cargo_capacity",
        value_num=456.0,
        unit="SCU",
        source="scw",
        game_version="4.10.3",
        ingest_run_id=run,
    )
    upsert_fact(
        conn,
        entity_id=ship,
        key="manufacturer",
        value_text="RSI",
        source="scw",
        game_version="4.10.3",
        ingest_run_id=run,
    )
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    term = upsert_entity(conn, kind="terminal", canonical_name="Lorville", game_version="4.10.3")
    replace_price(
        conn,
        commodity_id=gold,
        terminal_id=term,
        side="sell",
        price=250.0,
        source="uex",
        game_version="4.10.3",
        ingest_run_id=run,
        reported_at="2026-01-01T00:00:00+00:00",
    )
    conn.commit()
    conn.close()


@pytest.fixture
def conn(tmp_path: Path):
    db = tmp_path / "a.sqlite"
    _seed(db)
    c = connect(db)
    yield c
    c.close()


def test_facts_cargo_brief(conn):
    r = answer_question(conn, "卡拉克货舱多大", detail_level="brief")
    assert r["intent"] == "facts"
    assert r["browser"]["needed"] is False
    assert "456" in r["answer_draft"] or "货舱" in r["answer_draft"]
    assert r["detail_available"] is True
    assert len(r["evidence"]["facts"]) <= 5
    assert r["plan"]["path"] == "mcp_only"


def test_facts_full_thicker(conn):
    brief = answer_question(conn, "卡拉克", detail_level="brief")
    full = answer_question(conn, "卡拉克", detail_level="full")
    assert len(full["evidence"]["facts"]) >= len(brief["evidence"]["facts"])


def test_prices_sell(conn):
    r = answer_question(conn, "黄金在哪卖")
    assert r["intent"] == "prices"
    assert r["browser"]["needed"] is False
    assert "Lorville" in r["answer_draft"] or "250" in r["answer_draft"]
    assert r["evidence"]["prices"]


def test_loadout_static_suggests_browser(conn):
    r = answer_question(conn, "卡拉克怎么配打炮")
    assert r["intent"] == "loadout_calc"
    assert "Erkul" in r["answer_draft"] or "配装" in r["answer_draft"]
    assert r["browser"]["needed"] is True
    assert r["browser"]["search_term"] == "Carrack"


def test_unknown_no_invent(conn):
    r = answer_question(conn, "完全不存在的飞船XYZ123货舱")
    assert "未覆盖" in r["answer_draft"] or "没有" in r["answer_draft"]
    assert r["detail_available"] is False
```

- [ ] **Step 2: Run tests — expect fail**

Run: `.venv/bin/python -m pytest tests/test_answer.py -v`

Expected: `ModuleNotFoundError` for `sc_search.query.answer`.

- [ ] **Step 3: Implement `answer.py`**

Create `src/sc_search/query/answer.py`:

```python
from __future__ import annotations

import sqlite3

from sc_search.query.entities import entity_facts
from sc_search.query.plan import query_plan
from sc_search.query.prices import best_prices
from sc_search.query.search import search_docs

_CARGO_KEYS = ("cargo_capacity", "cargo", "scu", "货舱")
_BRIEF_FACT_LIMIT = 5
_FULL_FACT_LIMIT = 25
_BRIEF_PRICE_LIMIT = 3
_FULL_PRICE_LIMIT = 10
_BRIEF_DOC_LIMIT = 2
_FULL_DOC_LIMIT = 5


def _empty_evidence() -> dict:
    return {"entities": [], "facts": [], "prices": [], "docs": [], "missing": []}


def _display_name(entities: list[dict]) -> str:
    if not entities:
        return ""
    e = entities[0]
    zh = e.get("zh")
    name = e["canonical_name"]
    if zh:
        return f"{zh}（{name}）"
    return str(name)


def _fact_value(f: dict) -> str:
    if f.get("value_num") is not None:
        unit = f.get("unit") or ""
        num = f["value_num"]
        if isinstance(num, float) and num.is_integer():
            num = int(num)
        return f"{num}{(' ' + unit) if unit else ''}".strip()
    return str(f.get("value_text") or "")


def _pick_facts(facts: list[dict], question: str, limit: int) -> list[dict]:
    q = (question or "").lower()
    scored: list[tuple[int, dict]] = []
    for f in facts:
        key = str(f.get("key") or "").lower()
        score = 0
        if any(k in key or k in q for k in _CARGO_KEYS) and (
            "cargo" in key or "货舱" in q or "cargo" in q
        ):
            score += 10
        if key in q or any(tok and tok in key for tok in q.split() if len(tok) > 1):
            score += 3
        if score:
            scored.append((score, f))
    scored.sort(key=lambda x: -x[0])
    picked = [f for _, f in scored[:limit]]
    if not picked:
        picked = facts[:limit]
    return [
        {
            "key": f.get("key"),
            "value_text": f.get("value_text"),
            "value_num": f.get("value_num"),
            "unit": f.get("unit"),
            "source": f.get("source"),
            "game_version": f.get("game_version"),
        }
        for f in picked
    ]


def _slim_prices(rows: list[dict]) -> list[dict]:
    return [
        {
            "commodity": r.get("commodity"),
            "terminal": r.get("terminal"),
            "side": r.get("side"),
            "price": r.get("price"),
            "game_version": r.get("game_version"),
            "source": r.get("source"),
        }
        for r in rows
    ]


def answer_question(
    conn: sqlite3.Connection,
    question: str,
    detail_level: str = "brief",
    hint_entity: str | None = None,
) -> dict:
    level = (detail_level or "brief").strip().lower()
    if level not in {"brief", "full"}:
        level = "brief"

    plan = query_plan(conn, question, hint_entity=hint_entity)
    intent = plan["intent"]
    evidence = _empty_evidence()
    evidence["entities"] = list(plan.get("entities") or [])
    follow_ups: list[str] = []
    detail_available = False
    draft = ""
    missing: list[str] = []

    fact_limit = _BRIEF_FACT_LIMIT if level == "brief" else _FULL_FACT_LIMIT
    price_limit = _BRIEF_PRICE_LIMIT if level == "brief" else _FULL_PRICE_LIMIT
    doc_limit = _BRIEF_DOC_LIMIT if level == "brief" else _FULL_DOC_LIMIT

    primary = evidence["entities"][0]["canonical_name"] if evidence["entities"] else ""
    label = _display_name(evidence["entities"])

    if intent in {"facts", "loadout_calc", "mining_calc"} and primary:
        bundle = entity_facts(conn, primary)
        if bundle:
            all_facts = bundle.get("facts") or []
            picked = _pick_facts(all_facts, question, fact_limit)
            evidence["facts"] = picked
            detail_available = len(all_facts) > len(picked) or level == "brief"
            ver = (bundle.get("entity") or {}).get("game_version") or ""
            if intent == "facts":
                if picked:
                    top = picked[0]
                    draft = (
                        f"{label} 的 {top.get('key')} 为 {_fact_value(top)}"
                        f"{('（' + ver + '）') if ver else ''}。"
                    )
                    if detail_available:
                        draft += "需要更细的硬点/部件可以说一声。"
                        follow_ups = ["展开全部参数", "只要货舱"]
                else:
                    draft = f"本地库有 {label}，但未找到匹配字段。"
                    missing.append("matching_facts")
            else:
                site = "Erkul" if intent == "loadout_calc" else "Regolith"
                static = ""
                if picked:
                    static = f"静态参考：{picked[0].get('key')}={_fact_value(picked[0])}。"
                draft = (
                    f"{label} 配装/计算需 {site}。"
                    f"{static}"
                    "本地只能给静态参数；要 DPS/配装结果再说，我再按指引开工具站。"
                )
                follow_ups = [f"打开 {site} 查配装", "只要静态参数"]
                detail_available = True
        else:
            missing.append("entity_facts")
            draft = f"未找到 {label or primary} 的本地事实。"

    elif intent == "prices":
        side = "sell"
        for step in plan.get("mcp_steps") or []:
            if step.get("tool") == "sc_best_prices":
                side = (step.get("args") or {}).get("side") or "sell"
        commodity = primary or question
        rows = best_prices(conn, commodity, side=side, limit=price_limit)
        evidence["prices"] = _slim_prices(rows)
        if rows:
            top = rows[0]
            draft = (
                f"{top.get('commodity')} {side} 较优：{top.get('terminal')} "
                f"约 {top.get('price')} aUEC"
                f"{('（' + str(top.get('game_version')) + '）') if top.get('game_version') else ''}。"
            )
            detail_available = True
            follow_ups = ["看更多终端", "查买入价"] if side == "sell" else ["看更多终端", "查卖出价"]
        else:
            missing.append("prices")
            draft = f"本地没有 {commodity} 的 {side} 报价。"
            detail_available = False

    elif intent == "docs":
        rows = search_docs(conn, question, limit=doc_limit)
        evidence["docs"] = [
            {"title": r.get("title"), "snippet": (r.get("snippet") or r.get("text") or "")[:240]}
            for r in rows
        ]
        if rows:
            snip = evidence["docs"][0].get("snippet") or ""
            draft = f"资料摘要：{snip[:120]}…" if snip else "找到相关文档。"
            detail_available = True
            follow_ups = ["展开更多 Wiki 摘要"]
        else:
            missing.append("docs")
            draft = "本地 Wiki 摘要未命中。"
            detail_available = False

    elif intent == "live_web":
        draft = "这类信息需要官网或 Spectrum；本地库不覆盖实时 Patch。需要的话再开浏览器。"
        detail_available = False
        follow_ups = ["打开 RSI 查 Patch Notes"]

    else:
        # low-confidence / unresolved facts path
        if not primary:
            draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
            missing.append("entity")
            detail_available = False
        elif not draft:
            draft = "暂无法根据本地数据作答。"
            detail_available = False

    evidence["missing"] = missing
    if not draft:
        draft = "本地知识库未覆盖该问题；不会编造数据。"

    return {
        "intent": intent,
        "path": plan["path"],
        "confidence": plan["confidence"],
        "answer_draft": draft,
        "evidence": evidence,
        "detail_available": detail_available,
        "follow_ups": follow_ups,
        "browser": plan.get("browser") or {"needed": False},
        "plan": plan,
    }
```

Adjust `search_docs` result keys to match actual `search.py` field names when implementing (read `src/sc_search/query/search.py` and map title/snippet correctly).

- [ ] **Step 4: Run tests — expect pass**

Run: `.venv/bin/python -m pytest tests/test_answer.py -v`

Expected: all PASSED. If `test_unknown` hits `facts` with empty entities, draft must still contain 未覆盖/没有.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add src/sc_search/query/answer.py tests/test_answer.py
git commit -m "$(cat <<'EOF'
feat: add answer_question one-shot local draft + evidence

EOF
)"
```

---

### Task 2: MCP `sc_answer`

**Files:**
- Modify: `src/sc_search/mcp_server/server.py`
- Modify: `tests/test_mcp_tools.py`

**Interfaces:**
- Consumes: `answer_question(conn, question, detail_level="brief", hint_entity=None) -> dict`
- Produces: `sc_answer(question: str, detail_level: str = "brief", hint_entity: str | None = None) -> str`; `handlers["sc_answer"]`

- [ ] **Step 1: Extend MCP test**

In `_seed` of `tests/test_mcp_tools.py`, ensure Carrack + cargo fact exist (or reuse ship seed from Task 1 pattern). Add:

```python
    ans = json.loads(mcp_server.handlers["sc_answer"]("卡拉克货舱多大"))
    assert ans["ok"] is True
    assert ans["data"]["browser"]["needed"] is False
    assert ans["data"]["answer_draft"]
```

- [ ] **Step 2: Run — expect fail**

Run: `.venv/bin/python -m pytest tests/test_mcp_tools.py::test_mcp_handlers -v`

Expected: KeyError `sc_answer`.

- [ ] **Step 3: Register tool**

In `server.py`:

```python
from sc_search.query.answer import answer_question

@mcp.tool()
def sc_answer(
    question: str,
    detail_level: str = "brief",
    hint_entity: str | None = None,
) -> str:
    """One-shot: classify + local DB fetch + short Chinese draft (no Erkul scrape)."""
    try:
        conn = open_db()
        try:
            data = answer_question(
                conn, question, detail_level=detail_level, hint_entity=hint_entity
            )
            meta = {}
            facts = (data.get("evidence") or {}).get("facts") or []
            prices = (data.get("evidence") or {}).get("prices") or []
            if facts:
                meta["game_version"] = facts[0].get("game_version")
            elif prices:
                meta["game_version"] = prices[0].get("game_version")
            return _ok(data, meta)
        finally:
            conn.close()
    except Exception as exc:
        return _err(str(exc))
```

Add `"sc_answer": sc_answer` to `handlers`.

- [ ] **Step 4: Run tests**

Run: `.venv/bin/python -m pytest tests/test_answer.py tests/test_mcp_tools.py -v`

Expected: all PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add src/sc_search/mcp_server/server.py tests/test_mcp_tools.py
git commit -m "$(cat <<'EOF'
feat: expose sc_answer MCP tool

EOF
)"
```

---

### Task 3: AGENTS.md + README

**Files:**
- Modify: `AGENTS.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: tool name `sc_answer` and fields from Task 1
- Produces: Agent rules only

- [ ] **Step 1: Update AGENTS MCP bullets**

Replace the「强制顺序」/`sc_query_plan` 默认入口为：

```markdown
  * **默认入口**：事实类先 `sc_answer(question)`（`detail_level` 默认 `brief`）→ 优先转发 `answer_draft`；需要细节时再 `sc_answer(..., detail_level="full")` 或 `sc_entity_facts`。
  * **浏览器门禁**：`path=mcp_only` 且高置信 → 禁止开浏览器；`loadout_calc`/`mining_calc` 先给静态短答，仅当用户要配装结果或明确核对时才按返回的 `browser` 打开；`confidence=low` 不默认连开浏览器。
  * `sc_query_plan` 可选（只要路由/调试时）。
  * 工具：`sc_answer`、`sc_query_plan`、`sc_resolve_name`、`sc_best_prices`、`sc_terminal_prices`、`sc_data_freshness`、`sc_entity_facts`、`sc_search_docs`。
```

Keep「不要编造库中没有的数据」。

- [ ] **Step 2: Align section 6**

Ensure section 6 still says browser only when plan/`sc_answer` `browser.needed` and user needs calc/confirm — not for verifying high-confidence local drafts.

- [ ] **Step 3: README**

Add `` `sc_answer` `` to the MCP tool list:「一键：归类+查库+短答草稿」。

- [ ] **Step 4: Re-run tests**

Run: `.venv/bin/python -m pytest tests/test_answer.py tests/test_mcp_tools.py -v`

Expected: PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add AGENTS.md README.md
git commit -m "$(cat <<'EOF'
docs: make sc_answer the default SC fact entry

EOF
)"
```

---

## Plan self-review

| Spec section | Task |
| :--- | :--- |
| One-shot flow + non-goals | Task 1–2 |
| API brief/full + evidence shape | Task 1 |
| Browser gates in AGENTS | Task 3 |
| Intent fetch table | Task 1 |
| Acceptance cases | Task 1 tests + Task 2 smoke |

No TBD placeholders. Signature consistent: `answer_question(...) -> dict`, MCP `sc_answer(...) -> str`.

---

## Execution handoff

Plan saved to `docs/superpowers/plans/2026-11-21-sc-answer-oneshot.md`.
