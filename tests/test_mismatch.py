from sc_search.worker.mismatch import check_mismatch, replan_suggested


def test_trade_with_only_docs_mismatches() -> None:
    rows = [
        {
            "id": "d1",
            "tool": "sc_search_docs",
            "ok": True,
            "data": [{"title": "Gold"}],
        }
    ]
    assert check_mismatch("trade", rows) is True
    assert replan_suggested(True, 0) is True
    assert replan_suggested(True, 1) is False


def test_trade_with_prices_ok() -> None:
    rows = [
        {
            "id": "p1",
            "tool": "sc_best_prices",
            "ok": True,
            "data": [{"price": 1.0}],
        }
    ]
    assert check_mismatch("trade", rows) is False


def test_empty_evidence_not_rule3() -> None:
    assert check_mismatch("trade", []) is False
    assert check_mismatch(
        "trade",
        [{"id": "x", "tool": "sc_best_prices", "ok": False, "data": None}],
    ) is False
