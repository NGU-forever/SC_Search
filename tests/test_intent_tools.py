from sc_search.mcp_server import server as mcp_server
from sc_search.query.intent_tools import (
    INTENT_TOOLS,
    MCP_TOOLS,
    playbook_for,
    validate_intent_tools,
)


def test_intent_tools_validate_clean() -> None:
    assert validate_intent_tools() == []


def test_mcp_tools_match_server_handlers() -> None:
    assert set(mcp_server.handlers) == set(MCP_TOOLS)


def test_agents_core_intents_present() -> None:
    required = {
        "trade",
        "ship_stats",
        "ship_buy",
        "mission_rep",
        "mission_money",
        "blueprint",
        "crafting",
        "loadout",
        "mining",
        "bug_tip",
        "client_help",
        "howto",
        "earn_meta",
        "pledge_meta",
        "event_guide",
        "lore_docs",
        "patch_live",
        "trade_route",
        "other",
    }
    assert required <= set(INTENT_TOOLS)


def test_mission_rep_playbook_order() -> None:
    pb = playbook_for("mission_rep")
    assert pb is not None
    assert pb["tools"][:2] == ["sc_resolve_name", "sc_list_missions"]
    assert pb["path"] == "mcp_only"


def test_loadout_waits_for_browser() -> None:
    pb = playbook_for("loadout")
    assert pb is not None
    assert pb["path"] == "mcp_then_browser"
    assert "sc_entity_facts" in pb["tools"]
