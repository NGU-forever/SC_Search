# SC Search — Agent 入口

本工作区默认在 **《星际公民》（Star Citizen / SC）** 语境下工作。

## 事实问答（MCP 已可用时）

```text
事实题默认先调 sc_ask(question=用户原话)。
非事实题（问候、感谢、闲聊、「星际公民好玩吗」这类主观感受）不调用 sc_ask，直接自然对话；若误调返回 `status=not_fact`，同样直接对话，不引用本地库、不提 sync。
跨领域问题（如刷任务+卖货/路线/避坑）可补调关联工具（如 sc_best_prices）并顺手查阅 TIPS.md，拼齐证据再成文。
以本地证据为事实锚点，结合 SC 机制与用户上下文展开成可执行的玩家方案（勿机械复读 answer_draft）。
browser.wait_confirm=true 时先征得用户确认。
```

成文原则（事实严、方案放开）：

- **硬事实严格守库**：数值、价格、手续费、冷却、店名、材料、版本只能来自本地证据；未收录或单位有争议就诚实说明，`stale=true` 标注快照版本但不喧宾夺主，**绝不编造具体数字**。
- **方案与推导充分展开**：推荐结构——**一行粗体结论先行** → 关键数据/机制 → 基于证据的算账对比、路线与取舍 → 实操避坑（优先引用 [TIPS.md](TIPS.md)）→ 可选下一步细化；7 类场景模板见 [`docs/answer_templates.md`](docs/answer_templates.md)。即使库未精确命中（`no_match` / `partial`），也先说明缺失字段，再给通用 SC 机制思路与下一步（标明「通用思路，具体数值以当前 Live 为准」），不要只丢一句「未收录」就收工。

硬约束：

- 事实优先本地 MCP **`sc-search`** / **`sc_ask`**；禁止先空搜 / 先开 Erkul 再查库。
- 外搜 query 必须含 `星际公民` / `Star Citizen` / `SC`。
- 用户分享的站点立即追加到 [SOURCES.md](SOURCES.md)。
- `leave_open` 交互站摘要后把页面留给用户。
- **改装**：社区优先 [Erkul Browse](https://erkul.games/browse)；Reddit 可补充但须**最新**帖（对照当前 Live，旧版方案勿当现行）；先基于本地船参 + 通用配装思路成文，再问是否打开 Browse 或计算器。

开发说明（**非运行时真相源**）：[ORCHESTRATION.md](ORCHESTRATION.md)、[`docs/state_machine_orchestration_design.md`](docs/state_machine_orchestration_design.md)。机器可读剧本：[`go/internal/router/intent_tools.json`](go/internal/router/intent_tools.json)。

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
   - 读取 `$ROOT/config/mcp_config.json`；用 `client_hints.<key>.aliases` 匹配宿主，对不上用 `unknown`。
   - JSON `mcpServers`（Cursor / Claude / Kimi / Antigravity 等）：合并 `sc-search`，**绝对路径**。
   - ChatGPT / Codex：写 TOML `mcp_servers`（见 `toml_example`）。
   - ZCode：优先 `.zcode` 的 `mcp.servers`，或设置页粘贴。
   - 豆包等无文件 UI：用 `ui_table` 列表格。
   - **只依赖** `command` + `args` + `env`。`cwd` 不是 MCP stdio 契约的一部分。
4. 可选自检：`./go/sc-search ingest status`；或从任意目录 STDIO 握手（绝对路径 `command` + `SC_SEARCH_DB`）。
5. 收尾：请用户 **重载 MCP / 新开对话**；刷新数据再说「同步数据」（包内二进制跑 `sync_all`，不必装 Go）。

### B. 源码开发树（完整路径）

1. **检查 Go**：`go version`；若无 Go，优先引导去 [GitHub Releases](https://github.com/NGU-forever/archangel-mcp/releases) 下对应平台 zip；仅开发才需 Go 1.22+。
2. **构建**：
   ```bash
   cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search
   chmod +x "$ROOT/scripts/"*.sh 2>/dev/null || true
   ```
3. **可选 UEX**：需要实时物价且无 Token 时，打开 [UEX My Apps](https://uexcorp.space/api/apps)，写入 `config/.env`（可从 `config/.env.example` 复制）。**不要编造 Token**；用户说跳过则跳过 `sync uex`。
4. **建库并同步**：
   ```bash
   cd "$ROOT" && ./scripts/sync_all.sh
   # Windows: .\scripts\sync_all.ps1
   ```
5. **写入 MCP**：`./scripts/setup_mcp.sh`，再读 `config/mcp_config.json` 按当前客户端适配（同 A.3）。
6. 自检与收尾同 A。

禁止：未就绪时假装 MCP 可用；相对路径写入 mcp / `mcp_config`；Release 树上仍装 Go / 重同步；自己能跑终端却甩一长串命令给用户；假定所有宿主同一配置路径。

### MCP 运行时（Go）

```bash
cd go && go build -o sc-search ./cmd/sc-search   # 开发
./scripts/setup_mcp.sh && ./scripts/run_mcp.sh   # Release / 已有二进制
```

运行时只需 `command` + `args=mcp` + `SC_SEARCH_DB`（绝对路径）。打包：`./scripts/package_release.sh`；`v*` tag 触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)。Ingest：`./scripts/sync_all.sh`。详见 [`go/README.md`](go/README.md)。
