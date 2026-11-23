# SC Search — 星际公民本地问答助手

在 [豆包](https://www.doubao.com/)（豆包工作）或其它支持 MCP 的 AI 客户端里，用**本地数据库**快速查《星际公民》物价、任务/蓝图、飞船参数和中英俗称，不必每次都翻网页。

运行时为 **Go 二进制**（MCP + ingest），无需 Python。

---

## 最省事：对 Agent 说「初始化」

把本仓库用 Cursor / Claude Code / Codex 等 **能改本地文件、能跑终端** 的 Agent 打开后，直接发：

```text
初始化
```

Agent 会按 [AGENTS.md](AGENTS.md) 里的「初始化」剧本自动：检查 Go → 构建 `go/sc-search` → `ingest init` + 同步数据 → 写入本客户端的 MCP 配置（绝对路径）→ 告诉你如何重载连接器。

| 客户端 | Agent 能自动做完的 | 你可能还要点一下的 |
| :--- | :--- | :--- |
| Cursor / Claude Code / Codex 等 | 构建、同步、写 `mcp.json` | 重载 MCP / 新开对话 |
| 豆包工作等「只配连接器、不写仓库」 | 构建、同步，并打印连接器字段 | 把表格字段粘进自定义连接器 |

没有 Agent、或想自己敲命令：继续看下面「5 分钟快速开始」。

---

## 你需要准备什么

| 项目 | 说明 |
| :--- | :--- |
| 电脑 | macOS / Windows / Linux 均可 |
| Go | **1.22+**（用于构建 `go/sc-search`） |
| AI 客户端 | 推荐 [豆包](https://www.doubao.com/)；也可用 Cursor 等 STDIO MCP 客户端 |
| UEX 账号（可选） | 要查**实时跑商物价**时需要；只查任务/飞船/Wiki 可不配 |

---

## 5 分钟快速开始

下面命令在**项目根目录**执行（也可跳过本节，改对 Agent 说「初始化」）。

### 1. 构建

```bash
cd go && go build -o sc-search ./cmd/sc-search && cd ..
```

### 2. 配置（查物价时需要）

1. 打开 [UEX My Apps](https://uexcorp.space/api/apps)，登录并**创建一个 App**，复制 **App Access Token**。
2. 复制环境文件并填入 Token：

```bash
cp config/.env.example config/.env
```

```text
UEX_API_TOKEN=粘贴你的Token
```

`SC_SEARCH_DB` 可留空（默认 `data/sc_knowledge.sqlite`）。

### 3. 初始化并下载数据

```bash
# macOS / Linux
chmod +x scripts/sync_all.sh scripts/run_mcp.sh
./scripts/sync_all.sh
```

```powershell
# Windows PowerShell
.\scripts\sync_all.ps1
```

脚本会：`init` → `uex` → `scmdb` → `scw` → `wiki` → `aliases` → `localization`。  
未配置 `UEX_API_TOKEN` 时物价同步会跳过。

手动逐步：

```bash
./go/sc-search ingest init
./go/sc-search ingest sync uex
./go/sc-search ingest sync scmdb live
./go/sc-search ingest sync scw
./go/sc-search ingest sync wiki
./go/sc-search ingest sync aliases
./go/sc-search ingest sync localization
./go/sc-search ingest status
```

### 4. 接到豆包 / Cursor

1. 确认 MCP 能启动：

```bash
./scripts/run_mcp.sh
# 或 ./go/sc-search mcp
```

2. 豆包自定义连接器（STDIO，**本地电脑**）：

| 字段 | 填写 |
| :--- | :--- |
| 服务器名称 | `sc-search` |
| 传输类型 | **STDIO** |
| 命令 | `/绝对路径/SC_Search/go/sc-search`（或 `scripts/run_mcp.sh`） |
| 参数 | `mcp`（用脚本时可留空） |
| 环境变量 | `SC_SEARCH_DB` = 数据库绝对路径 |

Cursor：

```bash
mkdir -p .cursor
cp config/mcp.cursor.example.json .cursor/mcp.json
```

把绝对路径改成你的仓库路径后重载 MCP（或对 Agent 说「初始化」自动写）。

### 5. 开始提问

1. **简单**单实体静态度/价 → `sc_answer`
2. **复杂**题 → `primary` + slots → `sc_route` → `sc_worker_run` → 成文
3. `replan_suggested` / `slot_retry_suggested` 各最多再跑 1 次

完整编排见 [ORCHESTRATION.md](ORCHESTRATION.md)。

| 你想问的 | 大致会用到的工具 |
| :--- | :--- |
| 简单事实/物价 | `sc_answer` |
| 复杂题 | `sc_route` → `sc_worker_run` |
| 「黄金在哪卖贵？」 | `sc_best_prices` |
| 「大白鲨是哪艘船？」 | `sc_resolve_name` |
| 「Carrack 载货 / 厂家？」 | `sc_entity_facts` |
| 「红风 Junior 能接什么任务？」 | `sc_list_missions` |
| 「数据是什么时候同步的？」 | `sc_data_freshness` |

---

## 游戏更新后

```bash
./scripts/sync_all.sh
# 或只同步 PTU：./go/sc-search ingest sync scmdb ptu
```

MCP **不会**自动改库；同步请在终端手动执行。

---

## 现在能查什么 / 还不能指望什么

**本地库比较擅长：** 跑商价、飞船 aUEC 购点（UEX）、任务/蓝图（SCMDB）、飞船参数（SCW）、Wiki 摘要、汉化组译名、民间俗称。

**仍建议开网页确认：** Erkul 配装/DPS、采矿计算、Spectrum、需登录官方内容。

---

## 常见问题

**Q：没有 UEX Token 能用吗？**  
能。跳过 `sync uex` 即可。

**Q：MCP 里看不到 sc-search？**  
确认命令是 `go/sc-search` 的绝对路径、`args=["mcp"]`，并重载连接器。自检：`./scripts/run_mcp.sh`。

**Q：查询结果是空的？**  
`./go/sc-search ingest status`，空库先 `init` 再 `sync`。

**Q：我想加俗称？**  
编辑 `data/seed_aliases.json`，再 `./go/sc-search ingest sync aliases`。

---

## 给开发者 / Agent

- Agent 入口：[`AGENTS.md`](AGENTS.md)
- 问答编排：[`ORCHESTRATION.md`](ORCHESTRATION.md)
- Go 说明：[`go/README.md`](go/README.md)
- 站点清单：[`SOURCES.md`](SOURCES.md)
- 测试：`cd go && go test ./...`

---

## 许可证

本仓库**代码与自有文档**以 [MIT License](LICENSE) 发布。

| 来源 | 用途 | 许可要点 |
| :--- | :--- | :--- |
| [汉化组 global.ini](https://citizenwiki.cn/Localization) | `sync localization` 标准中文译名 | **CC BY-NC-SA 4.0**，仅个人非商用 |
| UEX / SCMDB / Star Citizen Wiki 等 | 物价、任务、飞船参数、摘要 | 以各站点条款为准 |

本项目与 Cloud Imperium Games / RSI 无官方关联；数据来自社区站点，仅供个人学习与游戏辅助。
