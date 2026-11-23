# SC Search — Agent 入口

本工作区默认在 **《星际公民》（Star Citizen / SC）** 语境下工作。

回答事实类问题前，**必须先阅读并按** [ORCHESTRATION.md](ORCHESTRATION.md) **执行**（Router → Worker → 成文 → 可选浏览器；单实体静态度/价可旁路 `sc_answer`）。机器可读检索契约见 [`go/internal/router/intent_tools.json`](go/internal/router/intent_tools.json)（与 `sc_intents` 一致）。

硬约束摘要：

- 事实优先本地 MCP **`sc-search`**；禁止先空搜 / 先开 Erkul 再查库。
- 复杂题：定 `primary`+slots → `sc_route` → `sc_worker_run`；`replan_suggested` 改意图，`slot_retry_suggested` 改检索词；各最多再规划 1 次。合法 primary 见 `sc_intents`。
- 外搜 query 必须含 `星际公民` / `Star Citizen` / `SC`。
- 用户分享的站点立即追加到 [SOURCES.md](SOURCES.md)。
- 库无字段不编造；Erkul 等交互站在 `leave_open` 时摘要后把页面留给用户继续改。

---

## 用户说「初始化」时（Agent 必跑）

触发词（任一即可）：`初始化` / `init` / `setup` / `帮我配置 MCP` / 首次使用且库或二进制明显缺失。

**目标**：本机可查询；MCP 能被当前客户端加载。按序执行，能自动做的不要甩给用户。

1. **定位仓库根**（含 `go/cmd/sc-search`、`scripts/sync_all.sh` 的目录）。记下绝对路径 `ROOT`。
2. **检查 Go**：`go version`；若无 Go，说明需安装 1.22+，并给出下载页；有则继续。
3. **构建**：
   ```bash
   cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search
   chmod +x "$ROOT/scripts/run_mcp.sh" "$ROOT/scripts/sync_all.sh" 2>/dev/null || true
   ```
4. **可选 UEX**：若用户要实时物价且尚无 Token，提示打开 [UEX My Apps](https://uexcorp.space/api/apps)，把 Token 写入 `config/.env`（可从 `config/.env.example` 复制）。**不要编造 Token**；用户说跳过则跳过 `sync uex`。
5. **建库并同步**（优先一键）：
   ```bash
   cd "$ROOT" && ./scripts/sync_all.sh
   # Windows: .\scripts\sync_all.ps1
   ```
   失败时再逐步：`./go/sc-search ingest init` → `sync scmdb live` → `scw` → `wiki` → `aliases` → `localization`（有 Token 再 `sync uex`）。最后 `./go/sc-search ingest status`。
6. **写入 MCP 配置**（绝对路径，勿留占位符）：
   - **Cursor**：写/更新 `.cursor/mcp.json`（模板 [`config/mcp.cursor.example.json`](config/mcp.cursor.example.json)；`.cursor/` 已 gitignore，勿提交本机路径）：
     ```json
     {
       "mcpServers": {
         "sc-search": {
           "command": "<ROOT>/go/sc-search",
           "args": ["mcp"],
           "cwd": "<ROOT>",
           "env": { "SC_SEARCH_DB": "<ROOT>/data/sc_knowledge.sqlite" }
         }
       }
     }
     ```
   - **Claude Code / 其它读项目 `.mcp.json` 的客户端**：在仓库根写入同样内容的 `.mcp.json`（或该产品文档指定的路径）。
   - **豆包工作等只能在 UI 填连接器的客户端**：Agent **无法替用户点 UI**。构建+同步仍要做完，然后用表格列出：名称 `sc-search`、STDIO、命令=`<ROOT>/go/sc-search`、参数=`mcp`、环境变量 `SC_SEARCH_DB=<ROOT>/data/sc_knowledge.sqlite`，请用户粘贴并保存。
7. **自检**（可选）：对 `./go/sc-search mcp` 做一次短时 STDIO 握手，或调用 `sc_data_freshness`（若宿主已挂上工具）。
8. **收尾告诉用户**：
   - 请 **重载 MCP / 新开对话** 后再提问；
   - 简单题可直接问事实；复杂题走 `sc_route` → `sc_worker_run`；
   - 游戏大版本更新后跑 `./scripts/sync_all.sh`（或再说一次「同步数据」）。

禁止：在未构建、未配路径时假装 MCP 已可用；把相对路径写进 mcp.json；要求用户手动抄一长串命令却自己能跑终端。

### MCP 运行时（Go）

```bash
cd go && go build -o sc-search ./cmd/sc-search
# Cursor MCP: command=<repo>/go/sc-search  args=["mcp"]
# 或 ./scripts/run_mcp.sh
```

Ingest：`./scripts/sync_all.sh` 或 `sc-search ingest init|status|sync …`。说明见 [`go/README.md`](go/README.md)。
