from sc_search.query.guides import browser_guide, guides_for, load_guides


def test_load_guides_has_core_categories() -> None:
    data = load_guides()
    for key in ("howto", "client", "earn_meta", "pledge", "event"):
        assert key in data
        assert data[key]


def test_guides_for_limit() -> None:
    rows = guides_for("earn_meta", limit=2)
    assert len(rows) == 2
    assert rows[0]["url"].startswith("http")


def test_browser_guide_shape() -> None:
    b = browser_guide("client")
    assert b["needed"] is True
    assert b["wait_confirm"] is True
    assert b["site"] == "guide"
    assert b["start_url"] == b["resources"][0]["url"]
