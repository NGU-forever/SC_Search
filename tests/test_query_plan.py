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
    gold = upsert_entity(conn, kind="commodity", canonical_name="Gold", game_version="4.10.3")
    upsert_alias(conn, gold, "黄金", "zh", "manual")
    skin = upsert_entity(
        conn, kind="item", canonical_name='CQ7 "Blacklist" Rifle', game_version="4.10.3"
    )
    upsert_alias(conn, skin, 'CQ7 "Blacklist" Rifle', "en", "manual")
    bp = upsert_entity(conn, kind="blueprint", canonical_name="CQ7 Rifle", game_version="4.10.3")
    upsert_alias(conn, bp, "CQ7 Rifle", "en", "manual")
    upsert_alias(conn, bp, "CQ7 步枪", "zh", "manual")
    fs9_skin = upsert_entity(
        conn, kind="blueprint", canonical_name='FS-9 "Blacklist" LMG', game_version="4.10.3"
    )
    upsert_alias(conn, fs9_skin, 'FS-9 "Blacklist" LMG', "en", "manual")
    fs9 = upsert_entity(conn, kind="blueprint", canonical_name="FS-9 LMG", game_version="4.10.3")
    upsert_alias(conn, fs9, "FS-9 LMG", "en", "manual")
    parallax = upsert_entity(
        conn,
        kind="blueprint",
        canonical_name="Parallax Energy Assault Rifle",
        game_version="4.10.3",
    )
    upsert_alias(conn, parallax, "Parallax Energy Assault Rifle", "en", "manual")
    upsert_alias(conn, parallax, "视差", "slang", "manual")
    upsert_alias(conn, parallax, "视差步枪", "slang", "manual")
    coda = upsert_entity(
        conn, kind="blueprint", canonical_name="Coda Pistol", game_version="4.10.3"
    )
    upsert_alias(conn, coda, "Coda Pistol", "en", "manual")
    upsert_alias(conn, eid, "卡拉克", "zh", "manual")
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
    assert p["mcp_steps"][-1]["args"]["side"] == "sell"


def test_prices_buy_side(conn):
    p = query_plan(conn, "黄金在哪买")
    assert p["intent"] == "prices"
    assert p["mcp_steps"][-1]["args"]["side"] == "buy"


def test_loadout_unresolved_no_chinese_search_term(conn):
    p = query_plan(conn, "怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["search_term"] == ""
    assert p["confidence"] in ("low", "medium")
    for step in p["mcp_steps"]:
        if step["tool"] == "sc_resolve_name":
            assert step["args"]["text"] != "怎么配打炮"


def test_mining_unresolved_no_chinese_search_term(conn):
    p = query_plan(conn, "采矿头怎么配")
    assert p["intent"] == "mining_calc"
    assert p["browser"]["search_term"] == ""
    assert p["confidence"] in ("low", "medium")
    for step in p["mcp_steps"]:
        if step["tool"] == "sc_resolve_name":
            assert step["args"]["text"] != "采矿头怎么配"


def test_loadout_zh(conn):
    p = query_plan(conn, "卡拉克怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["needed"] is True
    assert p["browser"]["start_url"] == "https://erkul.games/calculator"
    assert p["browser"]["search_term"] == "Carrack"
    assert p["browser"]["max_actions"] == 8
    assert p["browser"].get("leave_open") is True
    assert p["browser"].get("handoff_to_user") is True


def test_loadout_en(conn):
    p = query_plan(conn, "Carrack best DPS loadout")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["search_term"] == "Carrack"


def test_mining_calc(conn):
    p = query_plan(conn, "鼹鼠怎么配激光头")
    assert p["intent"] == "mining_calc"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["start_url"] == "https://redmonstergaming.com"
    assert "regolith.rocks" not in {
        r.get("url", "") for r in (p["browser"].get("resources") or [])
    }


def test_shortcode_cq7_blueprint_context_prefers_blueprint(conn):
    q = "cq7的蓝图怎么获取以及我要制造的话，材料怎么得。"
    p = query_plan(conn, q)
    assert p["entities"], "expected token resolve to find CQ7 entities"
    kinds = {e["kind"] for e in p["entities"]}
    assert "blueprint" in kinds
    assert p["entities"][0]["kind"] == "blueprint"
    assert p["entities"][0]["canonical_name"] == "CQ7 Rifle"


def test_name_token_parallax_blueprint(conn):
    # Skin item shares the Parallax prefix; base blueprint must still win.
    skin_item = upsert_entity(
        conn,
        kind="item",
        canonical_name='Parallax "ArcCorp" Energy Assault Rifle',
        game_version="4.10.3",
    )
    upsert_alias(conn, skin_item, 'Parallax "ArcCorp" Energy Assault Rifle', "en", "manual")
    conn.commit()
    p = query_plan(conn, "Parallax 蓝图哪里弄，材料怎么得")
    assert p["entities"][0]["kind"] == "blueprint"
    assert p["entities"][0]["canonical_name"] == "Parallax Energy Assault Rifle"


def test_slang_shicha_rifle_blueprint(conn):
    p = query_plan(conn, "视差步枪蓝图怎么获取？制造要什么材料？")
    assert p["entities"][0]["canonical_name"] == "Parallax Energy Assault Rifle"


def test_fs9_prefers_base_lmg_not_blacklist_skin(conn):
    for q in ("FS-9 蓝图怎么获取，制造材料呢", "FS-9 LMG 蓝图怎么获取，材料怎么得"):
        p = query_plan(conn, q)
        assert p["entities"][0]["canonical_name"] == "FS-9 LMG", q


def test_coda_name_token_blueprint(conn):
    p = query_plan(conn, "Coda 蓝图怎么获取，材料怎么得")
    assert p["entities"][0]["canonical_name"] == "Coda Pistol"


def test_kalake_resolves_carrack(conn):
    p = query_plan(conn, "卡拉克货舱多大")
    assert p["entities"]
    assert p["entities"][0]["canonical_name"] == "Carrack"


def test_prices_nali_mai_and_zui_pian_yi(conn):
    al = upsert_entity(conn, kind="commodity", canonical_name="Aluminum", game_version="4.10.3")
    upsert_alias(conn, al, "铝", "zh", "seed")
    conn.commit()
    for q in ("铝哪里买最便宜？", "铝的买入最低价是多少？"):
        p = query_plan(conn, q)
        assert p["intent"] == "prices", q
        assert p["mcp_steps"][-1]["tool"] == "sc_best_prices"
        assert p["mcp_steps"][-1]["args"]["side"] == "buy"
        assert any(e["canonical_name"] == "Aluminum" for e in p["entities"]), q


def test_prices_nali_mai_sell(conn):
    qn = upsert_entity(conn, kind="commodity", canonical_name="Quantainium", game_version="4.10.3")
    upsert_alias(conn, qn, "量子矿", "zh", "seed")
    conn.commit()
    p = query_plan(conn, "量子矿哪里卖最贵？")
    assert p["intent"] == "prices"
    assert p["mcp_steps"][-1]["args"]["side"] == "sell"


def test_ship_buy_price_not_forced_into_commodity_prices(conn):
    hh = upsert_entity(conn, kind="ship", canonical_name="Aegis Hammerhead", game_version="4.10.3")
    upsert_alias(conn, hh, "锤头鲨", "zh", "seed")
    conn.commit()
    p = query_plan(conn, "锤头鲨游戏里多少钱可以买？")
    assert p["intent"] == "facts"
    assert p["entities"][0]["canonical_name"] == "Aegis Hammerhead"


def test_mission_list_faction_intent(conn):
    fac = upsert_entity(
        conn, kind="faction", canonical_name="Red Wind Linehaul", game_version="4.10.3"
    )
    upsert_alias(conn, fac, "红风物流", "zh", "seed")
    conn.commit()
    p = query_plan(conn, "红风物流有哪些任务？")
    assert p["intent"] == "mission_list"
    assert p["path"] == "mcp_only"
    assert p["mcp_steps"][-1]["tool"] == "sc_list_missions"
    assert "Red Wind" in p["mcp_steps"][-1]["args"]["faction"]


def test_single_char_zh_alias_embedded(conn):
    al = upsert_entity(conn, kind="commodity", canonical_name="Aluminum", game_version="4.10.3")
    upsert_alias(conn, al, "铝", "zh", "seed")
    # Noise single-char must not beat longer ship slang.
    hh = upsert_entity(conn, kind="ship", canonical_name="Aegis Hammerhead", game_version="4.10.3")
    upsert_alias(conn, hh, "大", "zh", "noise")
    upsert_alias(conn, hh, "锤头鲨", "zh", "seed")
    conn.commit()
    p = query_plan(conn, "铝货舱不相关只是解析")
    assert any(e["canonical_name"] == "Aluminum" for e in p["entities"])
    p2 = query_plan(conn, "卡拉克货舱多大")
    assert p2["entities"][0]["canonical_name"] == "Carrack"
    # Grammar char 大 must not become primary
    assert p2["entities"][0].get("zh") != "大"


def test_fs9_nali_mai_prefers_base_blueprint(conn):
    p = query_plan(conn, "FS-9轻机枪哪里买？")
    assert p["entities"][0]["kind"] == "blueprint"
    assert p["entities"][0]["canonical_name"] == "FS-9 LMG"


def test_earn_meta_guide_browser_resources(conn):
    p = query_plan(conn, "新手怎么赚钱？")
    assert p["intent"] == "earn_meta"
    assert p["path"] == "mcp_then_browser"
    assert p["browser"]["needed"] is True
    assert p["browser"].get("wait_confirm") is True
    assert p["browser"]["resources"]
    assert p["browser"]["start_url"].startswith("http")


def test_client_help_intent(conn):
    p = query_plan(conn, "进不去游戏报错19000怎么办")
    assert p["intent"] == "client_help"
    assert p["browser"]["resources"]


def test_howto_intent(conn):
    p = query_plan(conn, "罗威尔怎么走到空港叫船")
    assert p["intent"] == "howto"


def test_mission_list_not_swallowed_by_earn_meta(conn):
    fac = upsert_entity(
        conn, kind="faction", canonical_name="Red Wind Linehaul", game_version="4.10.3"
    )
    upsert_alias(conn, fac, "红风物流", "zh", "seed")
    conn.commit()
    p = query_plan(conn, "红风物流有哪些任务？")
    assert p["intent"] == "mission_list"


def test_loadout_still_erkul(conn):
    p = query_plan(conn, "卡拉克怎么配打炮")
    assert p["intent"] == "loadout_calc"
    assert p["browser"]["site"] == "erkul"
    assert p["browser"].get("resources")