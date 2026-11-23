# SC Search — Agent 入口

本工作区默认在 **《星际公民》（Star Citizen / SC）** 语境下工作。

回答事实类问题前，**必须先阅读并按** [ORCHESTRATION.md](ORCHESTRATION.md) **执行**（Router → Worker → 成文 → 可选浏览器；单实体静态度/价可旁路 `sc_answer`）。机器可读检索契约见 [`src/sc_search/query/intent_tools.py`](src/sc_search/query/intent_tools.py) 的 `INTENT_TOOLS`。

硬约束摘要：

- 事实优先本地 MCP **`sc-search`**；禁止先空搜 / 先开 Erkul 再查库。
- 复杂题：定 `primary`+slots → `sc_route` → `sc_worker_run`；`replan_suggested` 改意图，`slot_retry_suggested` 改检索词；各最多再规划 1 次。
- 外搜 query 必须含 `星际公民` / `Star Citizen` / `SC`。
- 用户分享的站点立即追加到 [SOURCES.md](SOURCES.md)。
- 库无字段不编造；Erkul 等交互站在 `leave_open` 时摘要后把页面留给用户继续改。
