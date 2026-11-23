"""Regression tests for Worker soft signals:
- evidence_empty / slot_retry_suggested (empty evidence, NOT mismatch)
- mismatch / replan_suggested rules unchanged (rule-3 only, wrong family)
- docs_query falls back to full question for meta/colloquial activity slot
- sc_list_missions does NOT receive meta activity as rank_keyword
"""
from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest

from sc_search.router.dag import expand_dag
from sc_search.worker.executor import run_plan
from sc_search.worker.mismatch import (
    check_mismatch,
    evidence_is_empty,
    slot_retry_suggested,
    strong_family_missing,
)
from sc_search.worker.slots import docs_query, is_meta_keyword, mission_rank_keyword


# ---------------------------------------------------------------------------
# Unit: slots helpers
# ---------------------------------------------------------------------------

class TestIsMetaKeyword:
    """is_meta_keyword should recognise colloquial meta tokens."""

    @pytest.mark.parametrize(
        "text",
        ["开荒", "赚钱", "怎么赚", "萌新", "新手", "刷钱", "earn", "grind", "newbie"],
    )
    def test_meta_tokens(self, text: str) -> None:
        assert is_meta_keyword(text), f"{text!r} should be meta"

    @pytest.mark.parametrize(
        "text",
        ["Hauling", "货运", "Bounty", "Mining", "Trade", "Salvage"],
    )
    def test_non_meta_activity(self, text: str) -> None:
        assert not is_meta_keyword(text), f"{text!r} should NOT be meta"

    def test_empty_and_none(self) -> None:
        assert not is_meta_keyword(None)
        assert not is_meta_keyword("")


class TestDocsQuery:
    """docs_query falls back to full question when slot is meta."""

    def test_meta_slot_falls_back_to_question(self) -> None:
        q = "星际公民怎么赚钱，推荐给新手"
        assert docs_query("开荒", q) == q

    def test_meta_slot_earn_falls_back(self) -> None:
        q = "怎么赚第一桶金"
        assert docs_query("赚钱", q) == q

    def test_concrete_slot_used(self) -> None:
        q = "星际公民怎么赚钱"
        assert docs_query("Hauling", q) == "Hauling"

    def test_none_slot_falls_back(self) -> None:
        q = "some question"
        assert docs_query(None, q) == q

    def test_empty_slot_falls_back(self) -> None:
        q = "some question"
        assert docs_query("", q) == q


class TestMissionRankKeyword:
    """mission_rank_keyword skips meta candidates."""

    def test_meta_alone_returns_none(self) -> None:
        assert mission_rank_keyword("开荒") is None

    def test_meta_then_concrete_returns_concrete(self) -> None:
        assert mission_rank_keyword("萌新", "Hauling") == "Hauling"

    def test_concrete_first_returned(self) -> None:
        assert mission_rank_keyword("Bounty", "开荒") == "Bounty"

    def test_all_meta_returns_none(self) -> None:
        assert mission_rank_keyword("开荒", "赚钱", "新手") is None

    def test_none_values_skipped(self) -> None:
        assert mission_rank_keyword(None, None, "Mining") == "Mining"


# ---------------------------------------------------------------------------
# Unit: mismatch / soft signals
# ---------------------------------------------------------------------------

class TestEvidenceIsEmpty:
    def test_empty_list(self) -> None:
        assert evidence_is_empty([]) is True

    def test_none(self) -> None:
        assert evidence_is_empty(None) is True  # type: ignore[arg-type]

    def test_non_empty(self) -> None:
        assert evidence_is_empty([{"id": "x"}]) is False


class TestSlotRetrySuggested:
    """slot_retry_suggested: empty evidence OR strong_missing, no mismatch, replan_count < 1."""

    def test_empty_no_mismatch(self) -> None:
        assert slot_retry_suggested(evidence_empty=True, mismatch=False, replan_count=0) is True

    def test_empty_but_mismatch_suppresses(self) -> None:
        # mismatch takes priority; slot retry must NOT fire
        assert slot_retry_suggested(evidence_empty=True, mismatch=True, replan_count=0) is False

    def test_not_empty_suppresses(self) -> None:
        assert slot_retry_suggested(evidence_empty=False, mismatch=False, replan_count=0) is False

    def test_strong_missing_triggers(self) -> None:
        assert slot_retry_suggested(
            evidence_empty=False, mismatch=False, replan_count=0, strong_missing=True
        ) is True

    def test_already_retried(self) -> None:
        assert slot_retry_suggested(evidence_empty=True, mismatch=False, replan_count=1) is False


class TestStrongFamilyMissing:
    def test_mission_money_resolve_only(self) -> None:
        rows = [
            {
                "id": "r1",
                "tool": "sc_resolve_name",
                "ok": True,
                "data": [{"canonical_name": "Bit Zeros", "kind": "faction"}],
            },
            {
                "id": "c1",
                "tool": "sc_list_missions",
                "ok": True,
                "data": [],
                "meta": {"why": "no_match"},
            },
        ]
        assert strong_family_missing("mission_money", rows) is True

    def test_mission_money_with_missions(self) -> None:
        rows = [
            {"id": "c1", "tool": "sc_list_missions", "ok": True, "data": [{"name": "Heist"}]},
        ]
        assert strong_family_missing("mission_money", rows) is False


class TestMismatchRules:
    """check_mismatch must NOT fire on empty evidence."""

    def test_empty_evidence_never_mismatch(self) -> None:
        assert check_mismatch("trade", []) is False

    def test_all_failed_calls_not_mismatch(self) -> None:
        rows = [
            {"id": "p1", "tool": "sc_best_prices", "ok": False, "data": None},
        ]
        assert check_mismatch("trade", rows) is False

    def test_earn_meta_empty_not_mismatch(self) -> None:
        assert check_mismatch("earn_meta", []) is False

    def test_trade_only_docs_is_mismatch(self) -> None:
        """Rule-3: trade got only docs family → mismatch."""
        rows = [
            {"id": "d1", "tool": "sc_search_docs", "ok": True, "data": [{"title": "Gold guide"}]},
        ]
        assert check_mismatch("trade", rows) is True

    def test_earn_meta_docs_alone_is_weak_mismatch(self) -> None:
        """earn_meta has strong families {missions, answer}; docs alone is still
        a rule-3 mismatch (only-weak). Missions+docs together is NOT mismatch."""
        rows_docs_only = [
            {"id": "d1", "tool": "sc_search_docs", "ok": True, "data": [{"title": "Hauling guide"}]},
        ]
        # docs alone → weak_alone triggers mismatch for earn_meta
        assert check_mismatch("earn_meta", rows_docs_only) is True

    def test_earn_meta_missions_plus_docs_not_mismatch(self) -> None:
        """When missions (strong) also hit, the combo is acceptable."""
        rows = [
            {"id": "m1", "tool": "sc_list_missions", "ok": True, "data": [{"name": "Haul"}]},
            {"id": "d1", "tool": "sc_search_docs", "ok": True, "data": [{"title": "Hauling guide"}]},
        ]
        assert check_mismatch("earn_meta", rows) is False


# ---------------------------------------------------------------------------
# DAG expand: meta keyword NOT passed as rank_keyword; docs query uses question
# ---------------------------------------------------------------------------

class TestExpandDagMetaBehavior:
    """expand_dag: meta activity slot must not appear as rank_keyword."""

    @pytest.mark.parametrize(
        "question,activity",
        [
            ("怎么赚钱", "开荒"),
            ("萌新怎么赚第一桶金", "赚钱"),
            ("新手该干什么", "新手"),
        ],
    )
    def test_no_meta_rank_keyword(self, question: str, activity: str) -> None:
        plan = expand_dag(question, "earn_meta", {"activity": activity})
        mission_calls = [c for c in plan.calls if c["tool"] == "sc_list_missions"]
        for c in mission_calls:
            assert "rank_keyword" not in c["args"], (
                f"rank_keyword should NOT appear for meta activity={activity!r}, "
                f"got args={c['args']}"
            )

    def test_non_meta_activity_keeps_rank_keyword(self) -> None:
        plan = expand_dag("怎么赚钱", "earn_meta", {"activity": "Hauling"})
        mission_calls = [c for c in plan.calls if c["tool"] == "sc_list_missions"]
        # At least one mission call should carry rank_keyword=Hauling
        assert any(c["args"].get("rank_keyword") == "Hauling" for c in mission_calls), (
            "Non-meta activity 'Hauling' should produce rank_keyword"
        )

    def test_meta_activity_docs_uses_full_question(self) -> None:
        """When activity is meta, sc_search_docs query must be the full question."""
        question = "怎么赚钱，萌新指南"
        plan = expand_dag(question, "earn_meta", {"activity": "开荒"})
        docs_calls = [c for c in plan.calls if c["tool"] == "sc_search_docs"]
        assert docs_calls, "earn_meta must include sc_search_docs"
        for c in docs_calls:
            assert c["args"].get("query") == question, (
                f"docs query should equal full question for meta activity, got {c['args'].get('query')!r}"
            )


# ---------------------------------------------------------------------------
# Integration: run_plan returns evidence_empty / slot_retry_suggested
# ---------------------------------------------------------------------------

def _make_null_handlers() -> dict[str, Any]:
    """Handlers that return ok=True but data=None (simulates total miss — no evidence)."""

    def _null_ok(**_kwargs: Any) -> str:
        return json.dumps({"ok": True, "data": None, "meta": {}})

    tools = [
        "sc_resolve_name", "sc_entity_facts", "sc_list_missions", "sc_locations",
        "sc_best_prices", "sc_terminal_prices", "sc_search_docs",
        "sc_data_freshness", "sc_answer", "sc_query_plan",
    ]
    return {t: _null_ok for t in tools}


def _make_docs_handlers(docs_data: list[dict]) -> dict[str, Any]:
    """Handlers: sc_search_docs returns data, all others empty."""
    null_h = _make_null_handlers()

    def _docs(**_kwargs: Any) -> str:
        return json.dumps({"ok": True, "data": docs_data, "meta": {}})

    return {**null_h, "sc_search_docs": _docs}


def _make_prices_handlers(prices_data: list[dict]) -> dict[str, Any]:
    null_h = _make_null_handlers()

    def _prices(**_kwargs: Any) -> str:
        return json.dumps({"ok": True, "data": prices_data, "meta": {}})

    return {**null_h, "sc_best_prices": _prices, "sc_terminal_prices": _prices}


class TestRunPlanSoftSignals:
    """run_plan must return evidence_empty / slot_retry_suggested correctly."""

    def test_earn_meta_all_empty_evidence_empty(self) -> None:
        """开荒 earn_meta: all tools return [], evidence_empty True, mismatch False."""
        plan = expand_dag("开荒怎么赚钱", "earn_meta", {"activity": "开荒"})
        out = run_plan(plan.to_dict(), _make_null_handlers())
        assert out["ok"] is True
        assert out["evidence_empty"] is True, "evidence should be empty when all tools return []"
        assert out["mismatch"] is False, "empty evidence must NOT trigger mismatch"
        assert out["slot_retry_suggested"] is True, "slot retry should be suggested when empty+no mismatch"
        assert out["replan_suggested"] is False, "replan must NOT be suggested (not a mismatch)"

    def test_earn_meta_with_missions_evidence_not_empty(self) -> None:
        """earn_meta with missions hit → evidence_empty False, no retry, no mismatch."""
        plan = expand_dag("星际公民怎么赚钱", "earn_meta", {"activity": "货运"})
        missions_data = [{"id": 1, "name": "Cargo run"}]
        null_h = _make_null_handlers()

        def _missions(**_kwargs: Any) -> str:
            return json.dumps({"ok": True, "data": missions_data, "meta": {}})

        handlers = {**null_h, "sc_list_missions": _missions}
        out = run_plan(plan.to_dict(), handlers)
        assert out["ok"] is True
        assert out["evidence_empty"] is False
        assert out["slot_retry_suggested"] is False
        assert out["mismatch"] is False

    def test_trade_only_docs_mismatch_not_slot_retry(self) -> None:
        """trade plan with only sc_search_docs hit → mismatch True, replan True, slot_retry False."""
        plan = expand_dag("黄金哪里卖", "trade", {"commodity": "黄金", "side": "sell"})
        # Manually inject a docs call to simulate wrong evidence
        docs_plan = plan.to_dict()
        docs_plan["calls"] = [
            {"id": "c1", "tool": "sc_search_docs", "args": {"query": "gold"}, "needs": []}
        ]
        handlers = _make_docs_handlers([{"title": "Gold mining guide"}])
        out = run_plan(docs_plan, handlers)
        assert out["ok"] is True
        assert out["mismatch"] is True, "trade with only docs should mismatch"
        assert out["replan_suggested"] is True
        assert out["slot_retry_suggested"] is False, "mismatch should suppress slot retry"
        assert out["evidence_empty"] is False

    def test_earn_meta_already_retried_no_slot_retry(self) -> None:
        """When replan_count >= 1, slot_retry_suggested must be False."""
        plan = expand_dag("开荒怎么赚钱", "earn_meta", {"activity": "开荒"})
        d = plan.to_dict()
        d["replan_count"] = 1
        out = run_plan(d, _make_null_handlers())
        assert out["evidence_empty"] is True
        assert out["slot_retry_suggested"] is False, "already retried → no further slot retry"

    def test_reasoning_hint_mentions_slot_retry(self) -> None:
        """When slot_retry_suggested, reasoning_hint should not say '证据类型与意图'."""
        plan = expand_dag("开荒", "earn_meta", {"activity": "开荒"})
        out = run_plan(plan.to_dict(), _make_null_handlers())
        if out["slot_retry_suggested"]:
            hint = out.get("reasoning_hint", "")
            # Should NOT be the replan hint (which is for mismatch)
            assert "证据类型与意图明显不符" not in hint, (
                "slot retry hint must differ from replan hint"
            )
            assert hint  # must have some text

    def test_earn_meta_non_meta_activity_haul_can_get_evidence(self) -> None:
        """earn_meta with Hauling (non-meta) → missions count as evidence when returned."""
        plan = expand_dag("怎么赚钱，推荐货运", "earn_meta", {"activity": "Hauling"})
        missions_data = [{"id": 1, "name": "Haul cargo"}]

        null_h = _make_null_handlers()

        def _missions(**_kwargs: Any) -> str:
            return json.dumps({"ok": True, "data": missions_data, "meta": {}})

        handlers = {**null_h, "sc_list_missions": _missions}
        out = run_plan(plan.to_dict(), handlers)
        assert out["ok"] is True
        assert out["evidence_empty"] is False, "Hauling missions should populate evidence"
        assert out["mismatch"] is False
        assert out["slot_retry_suggested"] is False

    def test_mission_money_resolve_hit_missions_no_match_slot_retry(self) -> None:
        """DeepSeek scenario: resolve hits faction but missions empty → slot_retry."""
        plan = expand_dag(
            "Bit Zeros ship heist",
            "mission_money",
            {"faction": "Bit Zeros", "activity": "Bit Zeros ship heist"},
        )

        def _resolve(**_kwargs: Any) -> str:
            return json.dumps(
                {
                    "ok": True,
                    "data": [{"canonical_name": "Bit Zeros", "kind": "faction"}],
                    "meta": {},
                }
            )

        def _missions_empty(**_kwargs: Any) -> str:
            return json.dumps(
                {
                    "ok": True,
                    "data": [],
                    "meta": {"why": "no_match", "message": "no missions matched"},
                }
            )

        def _null(**_kwargs: Any) -> str:
            return json.dumps({"ok": True, "data": None, "meta": {}})

        handlers = {
            "sc_resolve_name": _resolve,
            "sc_list_missions": _missions_empty,
            "sc_answer": _null,
            "sc_entity_facts": _null,
            "sc_best_prices": _null,
            "sc_terminal_prices": _null,
            "sc_search_docs": _null,
            "sc_data_freshness": _null,
            "sc_query_plan": _null,
        }
        out = run_plan(plan.to_dict(), handlers)
        assert out["mismatch"] is False
        assert out["strong_family_missing"] is True
        assert out["slot_retry_suggested"] is True
        assert any(f.get("tool") == "sc_list_missions" for f in out.get("failed_calls") or [])
        # Empty mission arrays must not pollute evidence
        assert all(
            (e.get("data") not in ([], {})) for e in out["evidence"]
        )
