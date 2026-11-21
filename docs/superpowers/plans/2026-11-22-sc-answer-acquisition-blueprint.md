# sc_answer acquisition (blueprint buy/price) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a buy/price question hits a `kind=blueprint` entity, `sc_answer` early-returns `evidence.acquisition` with `shop_buyable="unknown"` and a draft that surfaces blueprint/pool clues without inventing shop prices or asserting “绝对不可买”.

**Architecture:** Inline early-return in `answer_question` after `query_plan` resolves the primary entity: detect buy/price wording, if `kind=="blueprint"` load `entity_facts` for `pool_name`, build acquisition + draft, set `confidence=high` and `browser.needed=false`. Leave `query_plan` routing unchanged.

**Tech Stack:** Python 3, pytest, existing `sc_search.query.answer` / `entity_facts`.

**Spec:** `docs/superpowers/specs/2026-11-22-sc-answer-acquisition-blueprint-design.md`

## Global Constraints

- Do not change `query_plan` intent regexes.
- P0 default `shop_buyable` is `"unknown"` (string); do not implement delist seed → `"false"`.
- `unknown` draft must NOT contain「商店不可直接购买」.
- Do not append「要用浏览器核实吗？」on this path.
- Do not invent shop names or historical aUEC prices.
- Live suffix via existing `_draft_live_suffix`.
- Commits only when user explicitly asks (this run: user said 执行 — commit after green tests).

---

## File map

| File | Role |
| :--- | :--- |
| `src/sc_search/query/answer.py` | Buy/price detect, blueprint early-return, acquisition + draft |
| `tests/test_answer.py` | Seed JS-400 blueprint + acceptance tests |
| `AGENTS.md` | One bullet on acquisition drafting |

---

### Task 1: Failing tests for blueprint acquisition path

**Files:**
- Modify: `tests/test_answer.py`
- Test: `tests/test_answer.py`

**Interfaces:**
- Consumes: `answer_question(conn, question, detail_level=...)`
- Produces: assertions on `evidence.acquisition`, draft, confidence, browser

- [ ] **Step 1: Extend `_seed` with JS-400 blueprint**

In `_seed`, after existing entities, add:

```python
    bp = upsert_entity(
        conn, kind="blueprint", canonical_name="JS-400", game_version="4.10.3"
    )
    upsert_alias(conn, bp, "JS-400", "en", "manual")
    upsert_fact(
        conn,
        entity_id=bp,
        key="pool_name",
        value_text="BP_REWARDS_FullStrikeOnStationB",
        source="scmdb",
        game_version="4.10.3",
        ingest_run_id=run,
    )
```

- [ ] **Step 2: Write failing tests**

```python
def test_blueprint_buy_acquisition_unknown(conn):
    for q in ("JS-400 哪里买", "JS-400 多少钱", "JS-400 能买到吗"):
        r = answer_question(conn, q)
        acq = r["evidence"].get("acquisition")
        assert acq is not None
        assert acq["shop_buyable"] == "unknown"
        assert acq["mode"] == "blueprint_reward"
        assert acq["pool"] == "BP_REWARDS_FullStrikeOnStationB"
        draft = r["answer_draft"]
        assert "蓝图" in draft or "奖励" in draft
        assert "未找到商店报价" in draft
        assert "商店不可直接购买" not in draft
        assert r["confidence"] == "high"
        assert r["browser"]["needed"] is False
        assert not any("浏览器核实" in f for f in r["follow_ups"])


def test_blueprint_what_is_no_acquisition_early_return(conn):
    r = answer_question(conn, "JS-400 是什么")
    assert r["evidence"].get("acquisition") is None
```

Keep `test_prices_sell` unchanged (Gold path).

- [ ] **Step 3: Run tests to verify they fail**

Run: `uv run pytest tests/test_answer.py::test_blueprint_buy_acquisition_unknown tests/test_answer.py::test_blueprint_what_is_no_acquisition_early_return -v`

Expected: FAIL (no `acquisition` key / assertion errors)

---

### Task 2: Implement early-return in `answer.py`

**Files:**
- Modify: `src/sc_search/query/answer.py`

**Interfaces:**
- Consumes: `query_plan` entities, `entity_facts`, `live_game_version`
- Produces: `evidence.acquisition` dict; early return from `answer_question`

- [ ] **Step 1: Add helpers**

```python
import re

_OBTAIN_RE = re.compile(
    r"(哪里买|在哪买|能买到|买得到|多少钱|价格|报价|哪[买卖出]|买点|卖点|\bbuy\b|\bsell\b|\bprice\b|买)",
    re.I,
)

def _wants_obtain_or_price(question: str) -> bool:
    return bool(_OBTAIN_RE.search(question or ""))


def _pool_name_from_facts(facts: list[dict]) -> str | None:
    for f in facts:
        if str(f.get("key") or "") == "pool_name":
            v = f.get("value_text")
            return str(v) if v else None
    return None


def _acquisition_unknown(pool: str | None) -> dict:
    return {
        "mode": "blueprint_reward",
        "shop_buyable": "unknown",
        "pool": pool,
        "hint_zh": "本地仅见蓝图/奖励记录，未找到商店报价",
    }


def _draft_blueprint_unknown(label: str, pool: str | None, live: str, src_ver: str | None) -> str:
    pool_bit = f"（池：{pool}）" if pool else ""
    return (
        f"{label} 本地仅见蓝图/任务奖励记录{pool_bit}，未找到商店报价；"
        f"不能据此断言绝对买不到{_draft_live_suffix(live, src_ver)}。"
    )
```

- [ ] **Step 2: Early-return after plan / primary are known**

After `live = live_game_version(conn)` and `label`/`primary` are set, before the big `if intent in {"facts", ...}` block:

```python
    primary_kind = (evidence["entities"][0].get("kind") if evidence["entities"] else None)
    if _wants_obtain_or_price(question) and primary_kind == "blueprint" and primary:
        bundle = entity_facts(conn, primary) or {}
        all_facts = bundle.get("facts") or []
        pool = _pool_name_from_facts(all_facts)
        acq = _acquisition_unknown(pool)
        evidence["acquisition"] = acq
        evidence["facts"] = [_slim_fact(f) for f in all_facts[:fact_limit]]
        evidence["missing"] = []
        src_ver = (bundle.get("entity") or {}).get("game_version")
        draft = _draft_blueprint_unknown(label or primary, pool, live, src_ver)
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": "high",
            "live_game_version": live,
            "answer_draft": draft,
            "evidence": evidence,
            "detail_available": False,
            "follow_ups": [],
            "browser": {"needed": False},
            "plan": plan,
        }
```

Ensure `_empty_evidence` callers still work; acquisition is an extra key only on this path.

- [ ] **Step 3: Run tests**

Run: `uv run pytest tests/test_answer.py -v`

Expected: all PASS (including existing Gold / Carrack cases)

- [ ] **Step 4: AGENTS.md one bullet**

Under MCP / 成文规则, add roughly:

> 买/价问法若 `evidence.acquisition` 存在：成文须遵循 `shop_buyable`；为 `unknown` 时说明无商店证据与蓝图/池线索，禁止用未出现在证据中的旧店名/标价覆盖。

- [ ] **Step 5: Commit**

```bash
git add tests/test_answer.py src/sc_search/query/answer.py AGENTS.md docs/superpowers/plans/2026-11-22-sc-answer-acquisition-blueprint.md
git commit -m "$(cat <<'EOF'
feat: sc_answer acquisition unknown for blueprint buy/price

EOF
)"
```

---

## Spec coverage check

| Spec item | Task |
| :--- | :--- |
| Buy/price + blueprint early-return | Task 2 |
| `shop_buyable=unknown` + pool + hint_zh | Task 2 |
| Draft: 蓝图/奖励 + 未找到商店报价; no 不可直接购买 | Task 1–2 |
| confidence high, no soft verify | Task 1–2 |
| Non-buy question no forced acquisition | Task 1 |
| Gold path unchanged | Task 1 (existing test) |
| AGENTS optional bullet | Task 2 Step 4 |
| No query_plan / ingest changes | Global |
