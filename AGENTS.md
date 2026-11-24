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
   脚本只写入 `.mcp.json` 与 `.cursor/mcp.json`（绝对路径），**不编译、不改库**。
3. **豆包等工作端**：无法写 UI 时，用表格列出：名称 `sc-search`、STDIO、命令=`<ROOT>/go/sc-search`、参数=`mcp`、环境变量 `SC_SEARCH_DB=<ROOT>/data/sc_knowledge.sqlite`。
4. 可选自检：`./go/sc-search ingest status` 或短时 STDIO 握手。
5. 收尾：请用户 **重载 MCP / 新开对话**；需要刷新数据时再说「同步数据」（用包内二进制跑 `sync_all`，仍不必装 Go）。

### B. 源码开发树（完整路径）

1. **检查 Go**：`go version`；若无 Go，优先引导去 [GitHub Releases](https://github.com/NGU-forever/archangel-mcp/releases) 下对应平台 zip；仅开发才需安装 Go 1.22+。
2. **构建**：
   ```bash
   cd "$ROOT/go" && go build -o sc-search ./cmd/sc-search
   chmod +x "$ROOT/scripts/"*.sh 2>/dev/null || true
   ```
3. **可选 UEX**：若用户要实时物价且尚无 Token，提示打开 [UEX My Apps](https://uexcorp.space/api/apps)，写入 `config/.env`（可从 `config/.env.example` 复制）。**不要编造 Token**；用户说跳过则跳过 `sync uex`。
4. **建库并同步**：
   ```bash
   cd "$ROOT" && ./scripts/sync_all.sh
   # Windows: .\scripts\sync_all.ps1
   ```
5. **写入 MCP**：优先 `./scripts/setup_mcp.sh`（同上绝对路径规则）。
6. 自检与收尾同 A。

禁止：在未就绪时假装 MCP 已可用；把相对路径写进 mcp.json；Release 树上仍去装 Go / 重同步；要求用户手动抄一长串命令却自己能跑终端。

### MCP 运行时（Go）

```bash
# 开发
cd go && go build -o sc-search ./cmd/sc-search
# 或 Release / 已有二进制
./scripts/setup_mcp.sh
./scripts/run_mcp.sh
```

打包发版：`./scripts/package_release.sh`；打 `v*` tag 触发 [`.github/workflows/release.yml`](.github/workflows/release.yml)。  
Ingest：`./scripts/sync_all.sh` 或 `sc-search ingest …`。说明见 [`go/README.md`](go/README.md)。
