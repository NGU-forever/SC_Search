from sc_search.router.dag import expand_dag


def test_mission_rep_resolve_then_parallel() -> None:
    plan = expand_dag(
        "红风物流声望任务",
        "mission_rep",
        {"faction": "红风物流", "rank": "Junior"},
    )
    assert plan.primary == "mission_rep"
    by_id = {c["id"]: c for c in plan.calls}
    assert by_id["r1"]["tool"] == "sc_resolve_name"
    assert by_id["r1"]["needs"] == []
    dependents = [c for c in plan.calls if c["id"] != "r1"]
    assert len(dependents) >= 2
    for c in dependents:
        assert c["needs"] == ["r1"]
    # list_missions + entity_facts share the same wave (both only need r1)
    need_sets = {tuple(c["needs"]) for c in dependents}
    assert need_sets == {("r1",)}


def test_earn_meta_parallel_roots() -> None:
    plan = expand_dag("怎么赚钱", "earn_meta", {"activity": "货运"})
    tools = [c["tool"] for c in plan.calls]
    assert "sc_answer" not in tools
    assert "sc_list_missions" in tools
    assert "sc_search_docs" in tools
    assert all(c["needs"] == [] for c in plan.calls)


def test_earn_meta_drops_meta_rank_uses_question_for_docs() -> None:
    plan = expand_dag("萌新怎么赚钱", "earn_meta", {"activity": "开荒"})
    by_tool = {c["tool"]: c for c in plan.calls}
    assert "rank_keyword" not in (by_tool["sc_list_missions"].get("args") or {})
    assert by_tool["sc_search_docs"]["args"]["query"] == "萌新怎么赚钱"


def test_trade_skips_terminal_without_slot() -> None:
    plan = expand_dag("黄金哪卖贵", "trade", {"commodity": "黄金", "side": "sell"})
    tools = [c["tool"] for c in plan.calls]
    assert "sc_best_prices" in tools
    assert "sc_terminal_prices" not in tools
    assert "sc_answer" not in tools


def test_unknown_primary_falls_to_other() -> None:
    plan = expand_dag("随便问问", "not_a_real_intent", {})
    assert plan.primary == "other"
