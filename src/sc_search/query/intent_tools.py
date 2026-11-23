"""Agent-layer intent → MCP tool playbook (deterministic retrieve contract).

Client LLM picks ``primary`` + slots; ``sc_route`` expands this into a DAG
(``router.dag.expand_dag``); ``sc_worker_run`` executes waves in parallel.
``sc_answer`` remains the one-shot bypass for simple entity fact/price questions.
"""

from __future__ import annotations

from typing import Any

# MCP tool names exposed by sc_search.mcp_server (keep in sync with server.py).
MCP_TOOLS = frozenset(
    {
        "sc_answer",
        "sc_query_plan",
        "sc_resolve_name",
        "sc_best_prices",
        "sc_terminal_prices",
        "sc_data_freshness",
        "sc_entity_facts",
        "sc_list_missions",
        "sc_search_docs",
        "sc_route",
        "sc_worker_run",
    }
)

# Agent primary intent → retrieve playbook.
# tools: preferred call order (skip steps already satisfied by prior evidence).
# path: mcp_only | mcp_then_browser | browser_only
# plan_intent: matching sc_query_plan intent when applicable (debug / optional).
INTENT_TOOLS: dict[str, dict[str, Any]] = {
    "trade": {
        "slots": ["commodity", "side", "system", "terminal"],
        "tools": ["sc_answer", "sc_best_prices", "sc_terminal_prices"],
        "path": "mcp_only",
        "plan_intent": "prices",
        "notes": "单货物价优先 sc_answer；指定终端用 sc_terminal_prices；side 默认卖出语境看问句",
    },
    "ship_stats": {
        "slots": ["ship"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts"],
        "path": "mcp_only",
        "plan_intent": "facts",
        "notes": "静态度量（货舱/厂家等）；命中实体后可只留 entity_facts",
    },
    "ship_buy": {
        "slots": ["ship"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts", "sc_search_docs"],
        "path": "mcp_then_browser",
        "plan_intent": "ship_shop",
        "notes": "本地 aUEC/购点来自 UEX 时走 sc_answer；店址/租借以外链 guide 为准，确认后再开浏览器",
    },
    "mission_rep": {
        "slots": ["faction", "rank", "system", "goal"],
        "tools": ["sc_resolve_name", "sc_list_missions", "sc_entity_facts"],
        "path": "mcp_only",
        "plan_intent": "mission_list",
        "notes": "先 resolve 势力英文名；list_missions(faction, rank_keyword)；不编造声望点数/时薪",
    },
    "mission_money": {
        "slots": ["activity", "system", "faction"],
        "tools": ["sc_list_missions", "sc_resolve_name", "sc_answer"],
        "path": "mcp_only",
        "plan_intent": "mission_list",
        "notes": "刷钱任务列表；有势力则带 faction；报酬只引用库内字段",
    },
    "blueprint": {
        "slots": ["item", "blueprint"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts"],
        "path": "mcp_only",
        "plan_intent": "facts",
        "notes": "短码/俗称先 resolve(kind 偏 blueprint)；获取通道看 acquisition；材料配方来自 SCW/SCMDB 证据",
    },
    "crafting": {
        "slots": ["item", "blueprint", "materials"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts"],
        "path": "mcp_only",
        "plan_intent": "facts",
        "notes": "同 blueprint；矿点未收录则明说，禁止编造刷新点",
    },
    "loadout": {
        "slots": ["ship"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts"],
        "path": "mcp_then_browser",
        "plan_intent": "loadout_calc",
        "notes": "先静态 facts；用户要 DPS/配装结果再开 Erkul（search_term 用英文标准名）",
    },
    "mining": {
        "slots": ["ship", "laser"],
        "tools": ["sc_answer", "sc_resolve_name", "sc_entity_facts"],
        "path": "mcp_then_browser",
        "plan_intent": "mining_calc",
        "notes": "先静态；确认后再开 community_guides.mining（RedMonster/SCMINER/Cryon 等）",
    },
    "bug_tip": {
        "slots": ["topic"],
        "tools": ["sc_search_docs", "sc_answer"],
        "path": "mcp_then_browser",
        "plan_intent": "client_help",
        "notes": "优先 TIPS.md / 本地 docs；不足再用 client guides，不编造排障步骤",
    },
    "client_help": {
        "slots": ["topic"],
        "tools": ["sc_search_docs"],
        "path": "mcp_then_browser",
        "plan_intent": "client_help",
        "notes": "报错/进不去/汉化；本地证据+外链，确认后开教程",
    },
    "howto": {
        "slots": ["topic"],
        "tools": ["sc_search_docs", "sc_answer"],
        "path": "mcp_then_browser",
        "plan_intent": "howto",
        "notes": "操作/新手流程；guides howto + 可有的本地事实",
    },
    "earn_meta": {
        "slots": ["activity", "system"],
        "tools": ["sc_list_missions", "sc_answer", "sc_search_docs"],
        "path": "mcp_then_browser",
        "plan_intent": "earn_meta",
        "notes": "赚钱开荒；可列任务线索，禁止编造时薪；外链供核对",
    },
    "pledge_meta": {
        "slots": ["topic"],
        "tools": ["sc_search_docs"],
        "path": "mcp_then_browser",
        "plan_intent": "pledge_meta",
        "notes": "氪金/CCU/退款；库无账单，靠官方与社区指南",
    },
    "event_guide": {
        "slots": ["event"],
        "tools": ["sc_search_docs", "sc_data_freshness"],
        "path": "mcp_then_browser",
        "plan_intent": "event_guide",
        "notes": "活动时效强；本地摘要不足则确认后开 RSI/社区",
    },
    "lore_docs": {
        "slots": ["query"],
        "tools": ["sc_search_docs", "sc_answer"],
        "path": "mcp_only",
        "plan_intent": "docs",
        "notes": "设定/Wiki 摘要 FTS；高置信不开浏览器",
    },
    "patch_live": {
        "slots": [],
        "tools": ["sc_data_freshness"],
        "path": "browser_only",
        "plan_intent": "live_web",
        "notes": "补丁说明以官网为准；freshness 仅作库版本对照",
    },
    "trade_route": {
        "slots": ["commodity", "system"],
        "tools": ["sc_best_prices", "sc_answer"],
        "path": "mcp_then_browser",
        "plan_intent": "trade_route",
        "notes": "整条航线需规划站；本地可附单价证据",
    },
    "other": {
        "slots": [],
        "tools": ["sc_answer", "sc_resolve_name", "sc_query_plan"],
        "path": "mcp_only",
        "plan_intent": "facts",
        "notes": "兜底：先 sc_answer；不明时 sc_query_plan 看路由，勿空搜",
    },
}


def playbook_for(intent: str) -> dict[str, Any] | None:
    key = (intent or "").strip()
    return INTENT_TOOLS.get(key)


def validate_intent_tools() -> list[str]:
    """Return list of validation errors (empty if OK)."""
    errors: list[str] = []
    for intent, row in INTENT_TOOLS.items():
        tools = row.get("tools") or []
        if not isinstance(tools, list) or not tools:
            errors.append(f"{intent}: tools must be non-empty list")
            continue
        for t in tools:
            if t not in MCP_TOOLS:
                errors.append(f"{intent}: unknown tool {t!r}")
        path = row.get("path")
        if path not in {"mcp_only", "mcp_then_browser", "browser_only"}:
            errors.append(f"{intent}: bad path {path!r}")
    return errors
