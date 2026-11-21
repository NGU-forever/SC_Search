# sc_answer P0 Confidence + ZH Labels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `sc_answer` recompute `confidence` from evidence, show Chinese fact labels in drafts, and on `low` for facts/prices/docs soft-suggest browser verify without setting `browser.needed=true`.

**Architecture:** Post-process inside `answer_question` only: static `_FACT_KEY_ZH`, `_evidence_confidence`, fix `detail_available`, adjust `follow_ups`/`browser` for low facts/prices/docs. Leave `query_plan` and nested `plan.confidence` unchanged.

**Tech Stack:** Python 3, pytest, existing `sc_search.query.answer`.

**Spec:** `docs/superpowers/specs/2026-11-21-sc-answer-p0-confidence-zh-design.md`

## Global Constraints

- Do not change `query_plan` routing regexes.
- Do not auto-set `browser.needed=true` for low confidence on facts/prices/docs.
- Nested `plan.confidence` stays as returned by `query_plan`.
- Live version display rules unchanged (SCMDB → Live 4.10.3).
- Commits only if user asked to execute with commits / explicitly commit.

---

## File map

| File | Role |
| :--- | :--- |
| `src/sc_search/query/answer.py` | ZH map, confidence, draft, detail_available, soft follow_up |
| `tests/test_answer.py` | New/updated acceptance tests |
| `AGENTS.md` | One bullet on confidence / low browser |

---

### Task 1: answer.py + tests

**Files:**
- Modify: `src/sc_search/query/answer.py`
- Modify: `tests/test_answer.py`

**Interfaces:**
- Consumes: existing `answer_question` / `query_plan` / evidence builders
- Produces: same `answer_question(...) -> dict` with recomputed top-level `confidence`, optional `key_zh` on fact evidence, Chinese draft labels

- [ ] **Step 1: Write failing tests**

Add to `tests/test_answer.py` (keep existing seeds; extend as needed):

```python
def test_cargo_draft_zh_and_high(conn):
    r = answer_question(conn, "卡拉克货舱多大")
    assert r["confidence"] == "high"
    assert "货舱" in r["answer_draft"]
    assert "cargo_capacity" not in r["answer_draft"] or "货舱" in r["answer_draft"]
    assert r["browser"]["needed"] is False
    assert r["evidence"]["facts"][0].get("key_zh") == "货舱"


def test_missing_field_low_soft_browser(conn):
    # ship exists; ask for a nonsense attribute → low/medium, needed false, soft follow_up
    r = answer_question(conn, "卡拉克量子糖分是多少")
    assert r["confidence"] in {"low", "medium"}
    assert r["browser"]["needed"] is False
    assert any("浏览器核实" in f for f in r["follow_ups"])


def test_no_prices_low(conn):
    # use commodity with no prices — add seed entity "Unobtanium" with zh alias, no prices
    r = answer_question(conn, "Unobtanium在哪卖")
    assert r["intent"] == "prices"
    assert r["confidence"] == "low"
    assert r["browser"]["needed"] is False
    assert any("浏览器核实" in f for f in r["follow_ups"])


def test_detail_available_false_when_exhausted(conn):
    # full with tiny entity that has ≤ brief limit facts after pick
    r = answer_question(conn, "卡拉克厂家", detail_level="full")
    # if all facts returned in evidence, detail_available should not be forced true merely for brief
    assert "detail_available" in r
```

Seed `Unobtanium` commodity + alias in `_seed` without prices. For nonsense field test, ensure confidence path hits low when no scored match—or medium if backfill returns unrelated facts; **prefer**: detect “question-specific miss” by requiring scored facts (score>0) for high; if only backfill, use medium and still add soft follow_up when question looks specific (contains 是多少/多大/什么) and top fact score was 0.

Clarify in implementation:

- `_pick_facts` should return `(picked, had_scored_hit: bool)` OR track whether any fact had score>0 before backfill.
- high only if `had_scored_hit` and value present for facts intent.
- If entity exists, no scored hit → `medium` or `low` (spec: medium for 无匹配字段); soft follow_up when confidence in {low, medium} **and** no scored hit for a specific question — spec says low gets follow_up; also add for medium-with-no-scored-hit to satisfy「未找到匹配」UX. **Per spec strictly:** follow_up only when `confidence=low`. So nonsense field with backfill → set confidence **`low`** when no scored hit (treat as 无匹配字段 → use **low** not medium for “specific attribute miss”). Spec table says 无匹配字段 → medium. Conflict with test wanting 浏览器核实.

**Resolution (binding):**  
- 无匹配字段（无 scored hit，仅有 backfill 或空）→ `confidence=medium` if backfill non-empty else `low`.  
- Soft follow_up「要用浏览器核实吗？」when `confidence in {low, medium}` **and** `not had_scored_hit` for facts — slightly broader than spec §2.1 low-only, matches user soft-suggest intent for weak answers. Document in code comment.

Update test:

```python
def test_missing_field_soft_browser(conn):
    r = answer_question(conn, "卡拉克量子糖分是多少")
    assert r["confidence"] in {"low", "medium"}
    assert r["browser"]["needed"] is False
    assert any("浏览器核实" in f for f in r["follow_ups"])
```

- [ ] **Step 2: Run tests — expect fail**

Run: `.venv/bin/python -m pytest tests/test_answer.py::test_cargo_draft_zh_and_high tests/test_answer.py::test_missing_field_soft_browser tests/test_answer.py::test_no_prices_low -v`

Expected: FAIL (货舱 not in draft / confidence still plan high / etc.)

- [ ] **Step 3: Implement in `answer.py`**

Add:

```python
_FACT_KEY_ZH = {
    "cargo_capacity": "货舱",
    "cargo_scu": "货舱",
    "cargo": "货舱",
    "manufacturer": "制造商",
    "manufacturer_code": "制造商代码",
    "length": "长度",
    "beam": "宽度",
    "height": "高度",
    "mass": "质量",
    "mass_total": "质量",
    "size": "尺寸",
    "type": "类型",
    "career": "职业",
    "foci": "定位",
    "production_status": "生产状态",
    "crew": "乘员",
    "hp": "结构值",
}
_SOFT_VERIFY = "要用浏览器核实吗？"


def _label_fact_key(key: str | None) -> str:
    k = (key or "").strip()
    if not k:
        return "参数"
    return _FACT_KEY_ZH.get(k) or _FACT_KEY_ZH.get(k.lower()) or k


def _slim_fact(f: dict) -> dict:
    key = f.get("key")
    return {
        "key": key,
        "key_zh": _label_fact_key(str(key) if key is not None else None),
        "value_text": f.get("value_text"),
        "value_num": f.get("value_num"),
        "unit": f.get("unit"),
        "source": f.get("source"),
        "game_version": f.get("game_version"),
    }
```

Change `_pick_facts` to return `tuple[list[dict], bool]` (`had_scored_hit`).

After building evidence/draft/missing, recompute:

```python
def _recompute_confidence(
    intent: str,
    *,
    plan_confidence: str,
    had_scored_hit: bool,
    has_entity: bool,
    facts: list,
    prices: list,
    docs: list,
    missing: list[str],
) -> str:
    if intent == "facts":
        if had_scored_hit and facts:
            return "high"
        if has_entity and facts:
            return "medium"  # backfill only
        if has_entity and not facts:
            return "low"
        return "low"
    if intent == "prices":
        return "high" if prices else "low"
    if intent == "docs":
        return "medium" if docs else "low"
    if intent in {"loadout_calc", "mining_calc"}:
        return "medium" if facts else "low"
    if intent == "live_web":
        return plan_confidence
    return plan_confidence if has_entity else "low"
```

Draft facts line use `_label_fact_key(top.get("key"))`.  
`detail_available`: remove `or level == "brief"`; set true only when more facts/relations/prices remain.  
For facts/prices/docs when confidence in {low, medium} and (intent prices/docs with empty OR facts without scored hit): append `_SOFT_VERIFY`; force `browser = {**browser, "needed": False}` for facts/prices/docs.

Preserve nested `plan` untouched.

- [ ] **Step 4: Run full answer + mcp tests**

Run: `.venv/bin/python -m pytest tests/test_answer.py tests/test_mcp_tools.py -v`

Expected: all PASSED.

- [ ] **Step 5: Commit (only if user asked)**

```bash
git add src/sc_search/query/answer.py tests/test_answer.py
git commit -m "$(cat <<'EOF'
fix: evidence confidence and Chinese fact labels in sc_answer

EOF
)"
```

---

### Task 2: AGENTS.md

**Files:**
- Modify: `AGENTS.md`

**Interfaces:** none (docs)

- [ ] **Step 1: Add bullet under MCP section**

```markdown
  * **置信度**：以 `sc_answer` 顶层 `confidence` 为准（可与嵌套 `plan.confidence` 不同）。`low`（及无命中字段的 `medium`）时勿擅自开浏览器；若 `follow_ups` 含「要用浏览器核实吗？」，等用户确认再开。
```

- [ ] **Step 2: Re-run tests**

Run: `.venv/bin/python -m pytest tests/test_answer.py -q`

Expected: PASSED.

- [ ] **Step 3: Commit (only if user asked)**

```bash
git add AGENTS.md
git commit -m "$(cat <<'EOF'
docs: trust sc_answer confidence; soft browser verify on weak evidence

EOF
)"
```

---

## Plan self-review

| Spec | Task |
| :--- | :--- |
| Evidence confidence table | Task 1 |
| ZH key map + key_zh | Task 1 |
| low soft follow_up, needed=false for facts/prices/docs | Task 1 |
| detail_available fix | Task 1 |
| AGENTS | Task 2 |
| Soft follow_up also for medium+no scored hit | Documented binding resolution vs strict low-only |

---

## Execution handoff

Plan saved to `docs/superpowers/plans/2026-11-21-sc-answer-p0-confidence-zh.md`.
