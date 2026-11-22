# Agent 意图理解 + MCP 检索成文

日期：2026-11-22  
状态：待用户审阅  
选定：方案 **1 — Agent 层 LLM 理解**；MCP 只做确定检索与证据，不做内嵌大模型分类。  
前置：`sc_answer` oneshot / P0 置信度与中文标签；本地 SQLite + `sc-search` MCP。

---

## 1. 目标与非目标

### 1.1 目标

用户口语（任务/买船/Bug/蓝图/材料/声望等）走：

```
用户话
  → ① Agent LLM：按 AGENTS prompt 拆解意图 + 槽位（结构化）
  → ② MCP：按意图调用索引/库工具，返回证据（可测、无编造）
  → ③ Agent LLM：只依据证据整理成中文回复用户
```

解决：正则 `query_plan` 对「红枫/Junior/刷声望」等口语不稳；多轮工具乱跳导致慢。

### 1.2 非目标

- MCP 进程内调用 LLM / 不新增需 API Key 的 `sc_classify`。  
- 不爬 Erkul；浏览器仍按现有门禁。  
- 不删除现有 `sc_answer` / `sc_query_plan`（降为快路径或兜底）。  
- 第一期不要求「声望点数最优」完整算法（库无则诚实缺失 + 软建议核实）。

---

## 2. 主流程

| 阶段 | 谁 | 做什么 | 产出 |
| :--- | :--- | :--- | :--- |
| 理解 | Agent LLM | 判主意图、次意图、槽位；纠正常见俗称（红枫→红风） | 内部结构化理解（不必暴露给用户） |
| 检索 | MCP | 只调白名单工具；禁止空搜/先开浏览器 | `evidence` / `answer_draft` / facts |
| 成文 | Agent LLM | 用证据写短答；缺数据说明；Live 以 `live_game_version` 为准 | 用户可见回复 |
| 可选浏览器 | Agent | 仅 `browser.needed` 且用户要配装/确认核实 | 补充站外信息 |

**硬规则**

1. 成文**不得**使用未出现在 MCP 证据中的数值/任务名。  
2. 理解阶段**不得**调用浏览器或 WebSearch 当默认。  
3. 名称不确定时：先 `sc_resolve_name`（或带 hint 的 `sc_answer`），再检索。

---

## 3. 意图与槽位（Agent prompt 契约）

第一期主意图枚举（可多标签，但指定 **primary**）：

| intent | 含义 | 典型槽位 |
| :--- | :--- | :--- |
| `mission_rep` | 任务/声望/军衔档 | faction, rank, system, goal |
| `mission_money` | 刷钱任务 | activity, system |
| `trade` | 跑商物价 | commodity, side, system |
| `ship_buy` / `ship_stats` | 买船/船参数 | ship |
| `blueprint` / `crafting` | 蓝图/材料 | item, blueprint |
| `loadout` | 配装/DPS | ship |
| `mining` | 采矿配装/模拟 | ship, laser |
| `bug_tip` | Bug/防坑 | topic（优先 TIPS/本地 docs） |
| `lore_docs` | 设定/Wiki | query |
| `patch_live` | Patch/官网 | — |
| `other` | 其它 | — |

槽位示例（`mission_rep`）：

```json
{
  "primary": "mission_rep",
  "slots": {
    "faction": "红风物流",
    "faction_hint_en": "Red Wind Linehaul",
    "rank": "Junior",
    "goal": "upgrade_reputation_fast"
  },
  "needs_browser": false
}
```

俗称纠错表（AGENTS 维护，可同步 aliases）：`红枫物流` → `红风物流` / `Red Wind Linehaul`。

---

## 4. 意图 → MCP 调用表

权威副本：`AGENTS.md` 检索契约表 + `src/sc_search/query/intent_tools.py`（`INTENT_TOOLS`，有单测）。

| primary | 工具顺序 | path |
| :--- | :--- | :--- |
| `trade` | `sc_answer` → `sc_best_prices` → `sc_terminal_prices` | mcp_only |
| `ship_stats` | `sc_answer` → `sc_resolve_name` → `sc_entity_facts` | mcp_only |
| `ship_buy` | answer / resolve / facts / docs；购点外链需确认 | mcp_then_browser |
| `mission_rep` | `sc_resolve_name(faction)` → `sc_list_missions` → `sc_entity_facts` | mcp_only |
| `mission_money` | `sc_list_missions` → resolve →（可选）`sc_answer` | mcp_only |
| `blueprint` / `crafting` | `sc_answer` → resolve（偏 blueprint）→ `sc_entity_facts` | mcp_only |
| `loadout` / `mining` | 静态 answer/resolve/facts → 用户要结果再浏览器 | mcp_then_browser |
| `bug_tip` / `client_help` / `howto` / `earn_meta` / `pledge_meta` / `event_guide` / `trade_route` | docs/missions/answer + guides；确认后再开 | mcp_then_browser |
| `lore_docs` | `sc_search_docs` →（可选）`sc_answer` | mcp_only |
| `patch_live` | `sc_data_freshness` + 官网 | browser_only |
| `other` | `sc_answer` → resolve →（可选）`sc_query_plan` | mcp_only |

保留：`sc_answer` = 简单事实/物价快路径；复杂口语 = Agent 拆槽 + 上表多工具。

---

## 5. MCP 任务列表（已落地）

`sc_list_missions(faction?, rank_keyword?, system?, limit?)` 已提供；`mission_rep` 标准路径为 resolve → list_missions。声望**点数/效率**仍可能未入库 → 成文时诚实说明，不编造。

---

## 6. 与现有组件关系

| 组件 | 角色 |
| :--- | :--- |
| AGENTS.md | **主契约**：意图枚举、槽位、调用表、禁编造、浏览器门禁 |
| `sc_answer` | 简单问句快路径；复杂问句可作辅证 |
| `sc_query_plan` | 调试/可选；不再当口语理解主力 |
| SQLite | 唯一事实源；Live=SCMDB 4.10.3 |

---

## 7. 端到端例：红风 Junior 刷声望

1. Agent 理解：`mission_rep`，faction=红风物流（纠错自红枫），rank=Junior。  
2. `sc_resolve_name("红风物流")` → `Red Wind Linehaul`。  
3. （有 `sc_list_missions` 时）筛 Junior/Hauler/contractor=Red Wind；否则 facts + 说明缺口。  
4. LLM 成文：列可接任务类型与报酬参考 + Live 4.10.3；无声望点数则不编造。  
5. 若用户要「社区最优刷法」→ follow_up 核实后再浏览器。

---

## 8. 落地范围（审阅通过后）

**本期（文档/规则，可马上做）**

1. 重写 AGENTS「理解 → 检索 → 成文」三步与意图表。  
2. 俗称：`红枫物流` → 红风 / Red Wind Linehaul（seed aliases）。  
3. 流程图/README 一句说明架构。

**紧随（代码，可另开 plan）**

4. `sc_list_missions` + 单测。  
5. 可选：`sc_answer` 增加对 `mission_rep` 关键词的弱提示（不替代 Agent 理解）。

---

## 9. 验收（行为）

| 输入 | 期望 |
| :--- | :--- |
| 红枫物流 Junior 刷声望 | Agent 纠错为红风；先 MCP resolve/任务证据；不成交编造声望点数 |
| 黄金在哪卖 | 可直接 `sc_answer`；高置信短答 |
| 卡拉克怎么配 | 静态短答；未要求时不开 Erkul |
| 纯 Bug 防坑 | 优先本地 TIPS/docs |

---

## 10. 小结

你要的「LLM 认话 → 后端索引 → LLM 整理」**可以**，且应落在 **Agent prompt + 确定 MCP**，而不是把 LLM 塞进 MCP。现有 `sc_answer` 继续服务简单题；复杂口语靠意图表驱动工具选择。
