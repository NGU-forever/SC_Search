# SC Search — 问答编排（开发说明）

> **非运行时真相源。** Agent 默认只调 **`sc_ask(question=…)`**（Go 状态机：`go/internal/orchestrate`）。分类、旁路、route/worker、最多 1 次回退、成文草稿与浏览器门禁均在服务端完成。

## 权威来源

| 用途 | 位置 |
| :--- | :--- |
| 意图 / 槽位 / 工具 DAG / path | [`go/internal/router/intent_tools.json`](go/internal/router/intent_tools.json)（与 `sc_intents` 一致） |
| 状态机设计与契约 | [`docs/state_machine_orchestration_design.md`](docs/state_machine_orchestration_design.md) |
| Agent 入口 + 初始化 | [AGENTS.md](AGENTS.md) |

## 运行时数据流（已由 `sc_ask` 执行）

```text
question → Classify
         ├─ 简单静态度/价 → Answer（sc_answer 旁路）
         └─ 复杂题 → Route → Worker →（可选 1 次 replan / slot_retry）
         → Present（answer_draft / answer_blocks）
         → BrowserGate（wait_confirm / leave_open；MCP 不主动开浏览器）
```

## 调试细粒度工具

仅在调试或显式覆盖时手调：`sc_route` → `sc_worker_run`，或直接 `sc_answer`。合法 `primary` 以 `sc_intents` / `intent_tools.json` 为准，勿在 Markdown 里维护第二份剧本表。

回退信号：`replan_suggested`（改 primary/slots）；`slot_retry_suggested`（改检索词）。`sc_ask` 已自动各最多执行 1 次。

## 浏览器门禁（摘要）

- `mcp_only` + 高置信 → 不开浏览器。
- `mcp_then_browser` / `browser_only` / `wait_confirm=true` → 先成文，征得用户确认后再开。
- **loadout / mining**：先社区/主流草稿，用户确认后再开 Erkul 等计算器；`leave_open` 时摘要后把页面留给用户。
- 搜索词用证据里的**英文标准名**；外搜须含 `星际公民` / `Star Citizen` / `SC`。

## 成文

数字、店名、任务名等只能来自证据；缺则写「本地未收录」。勿把 UEX 旧 `game_version` 当成当前 Live（看 `meta.stale` / `sc_data_freshness`）。
