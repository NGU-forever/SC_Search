# SC Search — Agent 入口

本工作区默认在 **《星际公民》（Star Citizen / SC）** 语境下工作。

事实题默认调用 MCP **`sc_ask(question=用户原话)`**（服务端状态机：分类 → 旁路/route+worker → 最多 1 次回退 → 成文 + 浏览器门禁）。用返回的 `answer_draft` / `answer_blocks` 成文；`browser.wait_confirm=true` 时先征得用户确认。细粒度 `sc_route` / `sc_worker_run` / `sc_answer` 仅调试或显式覆盖。开发说明见 [ORCHESTRATION.md](ORCHESTRATION.md)（**非运行时真相源**）与 [`docs/state_machine_orchestration_design.md`](docs/state_machine_orchestration_design.md)。

硬约束摘要：

- 事实优先本地 MCP **`sc-search`** / **`sc_ask`**；禁止先空搜 / 先开 Erkul 再查库。
- 外搜 query 必须含 `星际公民` / `Star Citizen` / `SC`。
- 用户分享的站点立即追加到 [SOURCES.md](SOURCES.md)。
- 库无字段不编造；Erkul 等交互站在 `leave_open` 时摘要后把页面留给用户继续改。
- **改装 `loadout`**：先看 `sc_ask` 草稿；再问是否开 Erkul；禁止一听「改装」就开计算器。

---

## 用户说「初始化」时（Agent 必跑）

触发词（任一即可）：`初始化` / `init` / `setup` / `帮我配置 MCP` / 首次使用且库或二进制明显缺失。

**目标**：本机可查询；MCP 能被当前客户端加载。按序执行，能自动做的不要甩给用户。

### 0. 定位根目录并判定模式

定位 `ROOT`（含 `scripts/setup_mcp.sh` / `data/` 的目录；Release 包根有 `.sc-search-root`）。

**Release / 已就绪**（满足即可走快路径，**禁止**再 `go build` / `sync_all`）：

- 存在可执行的 `go/sc-search`（或 Windows 下 `go/sc-search.exe`），**且**
- 存在非空的 `data/sc_knowledge.sqlite`

否则为 **源码开发树**，走完整构建+同步。

### A. Release / 已就绪（快路径）

1. `chmod +x "$ROOT/scripts/setup_mcp.sh" "$ROOT/scripts/run_mcp.sh" 2>/dev/null || true`
2. 执行：
   ```bash
   cd "$ROOT" && ./scripts/setup_mcp.sh
   # Windows: .\scripts\setup_mcp.ps1
   ```
   脚本写入 **`config/mcp_config.json`**（权威、含本机绝对路径 + `client_hints`），并顺带写 `.mcp.json` / `.cursor/mcp.json`。**不编译、不改库**。模板见 [`config/mcp_config.example.json`](config/mcp_config.example.json)。
3. **按当前客户端适配 MCP**（宿主各异，禁止假设人人都用 Cursor）：
   - 读取 `$ROOT/config/mcp_config.json`（由 example 渲染；hints 含 cursor / claude_code / claude_desktop / chatgpt·Codex / deepseek / kimi / zcode / antigravity / doubao / vscode / windsurf 等）。
   - 用 `client_hints.<key>.aliases` 匹配当前宿主；对得上则按 `format` + `write_paths`/`global_paths` 写入或展示；对不上用 `unknown`。
   - JSON `mcpServers` 形（Cursor / Claude / Kimi / Antigravity 等）：合并 `sc-search` 条目，**绝对路径**。
   - ChatGPT / Codex：写 TOML `mcp_servers`（见 `toml_example`），不要硬套 JSON。
   - ZCode：优先 `.zcode` 的 `mcp.servers`（见 `native_shape`），或设置页粘贴。
   - 豆包等无文件 UI：用 `ui_table` 字段列表格。
   - **只依赖标准字段**：`command` + `args` + `env`。`cwd` 不是 MCP stdio 契约的一部分，不要写、也不要当作必需。
4. 可选自检：`./go/sc-search ingest status`；或从任意目录做 STDIO 握手（`command` 用绝对路径，仅设 `SC_SEARCH_DB`）。
5. 收尾：请用户 **重载 MCP / 新开对话**；需要刷新数据时再说「同步数据」（用包内二进制跑 `sync_all`，仍不必装 Go）。

### B. 源码开发树（完整路径）

1. **检查 Go**：`go version`；若无 Go，优先引导去 [GitHub Releases](https://github.com/NGU-forever/archangel-mcp/releases) 下对应平台 zip；仅开发才需安装 Go 1.22+。
2. **构建**：
```bash
cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search
chmod +x "$ROOT/scripts/"*.sh 2>/dev/null || true
```3. **可选 UEX**：若用户要实时物价且尚无 Token，提示打开 [UEX My Apps](https://uexcorp.space/api/apps)，写入 `config/.env`（可从 `config/.env.example` 复制）。**不要编造 Token**；用户说跳过则跳过 `sync uex`。
4. **建库并同步**：
   ```bash
   cd "$ROOT" && ./scripts/sync_all.sh
   # Windows: .\scripts\sync_all.ps1
   ```
5. **写入 MCP**：优先 `./scripts/setup_mcp.sh`，再读 `config/mcp_config.json` 按当前客户端适配（同 A.3）。
6. 自检与收尾同 A。

禁止：在未就绪时假装 MCP 已可用；把相对路径写进 mcp / `mcp_config`；Release 树上仍去装 Go / 重同步；要求用户手动抄一长串命令却自己能跑终端；假定所有用户同一套 Agent/客户端配置路径。

### MCP 运行时（Go）

```bash
# 开发
cd go && go build -o sc-search ./cmd/sc-search
# 或 Release / 已有二进制
./scripts/setup_mcp.sh
./scripts/run_mcp.sh
```

运行时只需 `command` + `args=mcp` + `SC_SEARCH_DB`（绝对路径）；二进制不再要求 cwd 落在仓库内。

打包发版：`./scripts/package_release.sh`；打 `v*` tag 触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)。  
Ingest：`./scripts/sync_all.sh` 或 `sc-search ingest …`。说明见 [`go/README.md`](go/README.md)。
