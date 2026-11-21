# Star Citizen 本地知识库（SQLite）设计

日期：2026-11-21  
状态：待用户审阅（已纳入 MCP 优先）  
范围：从 SOURCES / AGENTS 所列站点摄入数据 → 本地 SQLite → **MCP Server 工具**供本项目通用 Agent 直接调用；与浏览器按需查询分工。

## 1. 背景与目标

本工作区要让《星际公民》知识可被 **Cursor / T3 等通用 Agent 在本项目内直接调用**。第一期不靠全站网页爬虫，而是：

1. 优先接入有正式 API / JSON dump 的源，写入本地 SQLite；
2. 用 **MCP Server（stdio）** 暴露查询工具；Agent 默认调 MCP 查库；库未覆盖再用浏览器；
3. 按阶段落地：**A 跑商物价 → B 任务/蓝图 → C 实体参数与中英俗称**。

技术栈：**Python** + SQLite（含 FTS5）+ **MCP Python SDK**（`mcp`）。

Agent 接入方式选定为 **MCP**（非仅 CLI Skill）。CLI 仍保留给 ingest / 调试，但问答入口以 MCP tools 为准。

## 2. 已否决的方案

| 方案 | 否决原因 |
| :--- | :--- |
| 每源独立宽表镜像 | Agent 跨源拼查询成本高，俗称难统一 |
| 整包 JSON blob + json_extract | 难索引、难 FTS、不适合长期问答 |
| 全站 HTML 爬虫（含 Erkul/采矿站） | SPA/无公开 API/登录与 ToS 风险；维护成本高 |

选定：**统一知识库模型** — 各源 Adapter 拉取 → Normalize → 同一套表。

## 3. 源站策略

| 站点 | 策略 | 阶段 |
| :--- | :--- | :--- |
| UEX (`uexcorp.space` / API 2.0) | REST + Bearer；写入 prices / entities | A |
| SCMDB (`scmdb.net/data/…`) | 下载 `versions.json` + `merged-*.json`（可先 mirror） | B |
| `api.star-citizen.wiki` | 分页 vehicles/items 等结构化游戏数据 | C |
| CitizenWiki (`citizenwiki.cn`) | MediaWiki API；别名与中文词条 | C |
| SC Trade Tools | 可选；需 API token；可与 UEX 互补 | 后置 |
| CStone (`finder.cstone.space`) | 可选；`GetSearch` + 详情页 | 后置 |
| Erkul / Regolith / SCMINER / Cryon / RedMonster | **不入库**；浏览器按需 | — |
| RSI / Spectrum | **不入库**；浏览器 + 登录态按需 | — |
| SC Focus | 低优先，默认可跳过 | — |

合规：遵守各站 ToS/限流；SCMDB 原始数据以本机私用为主（许可证偏限制再分发）；Token 仅存 `.env`。

## 4. 数据模型

库文件：`data/sc_knowledge.sqlite`。原始 JSON 可选落在 `data/raw/`，不进主查询路径。

### 4.1 表

**`ingest_runs`**  
`id`, `source`, `game_version`, `started_at`, `finished_at`, `status` (`running`/`ok`/`failed`), `row_count`, `notes`

**`entities`**  
`id`, `kind` (`commodity`/`ship`/`item`/`location`/`terminal`/`mission`/`blueprint`/…), `canonical_name`, `slug`, `uuid` (可空), `game_version`, `updated_at`  
唯一：有 `uuid` 时 `(kind, uuid, game_version)`；否则 `(kind, canonical_name, game_version)`。

**`aliases`**  
`entity_id`, `alias`, `lang` (`en`/`zh`/`slang`), `source`

**`facts`**  
`entity_id`, `key`, `value_text`, `value_num` (可空), `unit` (可空), `source`, `game_version`, `ingest_run_id`

**`prices`**  
`commodity_entity_id`, `terminal_entity_id`, `side` (`buy`/`sell`), `price`, `scu_available` (可空), `reported_at`, `source`, `game_version`, `ingest_run_id`  
第一期：每个 `(commodity, terminal, side, source, game_version)` **只保留最新一条**。历史价表 `prices_history` 不做。

**`relations`**  
`from_id`, `to_id`, `rel_type` (`recipe_input`/`recipe_output`/`mission_reward`/`sold_at`/…), `qty` (可空), `meta_json` (可空), `source`, `game_version`

**`documents`**  
`entity_id` (可空), `title`, `url`, `body`, `source`, `game_version`, `ingest_run_id`

**`documents_fts`**  
FTS5，索引 `title` + `body`。

### 4.2 阶段与表的对应

| 阶段 | 写入 |
| :--- | :--- |
| A UEX | entities, prices, 少量 aliases, ingest_runs |
| B SCMDB | entities, facts, relations |
| C Wiki / CitizenWiki | entities, facts, aliases, documents + FTS |

## 5. 摄入流水线

### 5.1 目录

```
SC_Search/
  config/.env.example
  data/sc_knowledge.sqlite
  data/raw/
  .cursor/mcp.json          # 或项目约定的 MCP 注册（指向本 server）
  pyproject.toml
  src/
    sc_search/              # 可安装包名，避免裸 src 歧义
      ingest/
        __main__.py
        cli.py
        db.py
        schema.sql
        normalize.py
        adapters/
          uex.py
          scmdb.py
          scw_api.py
          citizenwiki.py
      query/
        resolve.py
        prices.py
        entities.py
        search.py
        freshness.py
      mcp_server/
        __main__.py         # python -m sc_search.mcp_server
        server.py           # FastMCP / mcp.server 注册 tools
```

包名：`sc_search`。ingest：`python -m sc_search.ingest …`；MCP：`python -m sc_search.mcp_server`。

### 5.2 CLI

```text
python -m sc_search.ingest init
python -m sc_search.ingest sync uex [--game-version VERSION]
python -m sc_search.ingest sync scmdb [--channel live|ptu]
python -m sc_search.ingest sync scw
python -m sc_search.ingest sync aliases
python -m sc_search.ingest sync all
python -m sc_search.ingest status
```

### 5.3 单次 sync

1. 插入 `ingest_runs`（running）  
2. Adapter `fetch` → `parse` → 中间 dataclass  
3. `normalize` upsert  
4. **整次 run 一个事务**；失败则回滚并标记 failed  
5. 成功则更新 row_count / finished_at  

幂等：entities/aliases upsert；prices 替换该 source + game_version 下受影响的最新快照行。

### 5.4 Adapter 约束

- **uex**：Bearer（`UEX_API_TOKEN`）；尊重官方 rate limit；指数退避；User-Agent 含 `SC_Search-ingest/0.1`  
- **scmdb**：先 mirror（若可用）再 `scmdb.net/data/`；大文件一次下载  
- **scw_api**：分页；温和间隔  
- **citizenwiki**：MediaWiki 礼貌延迟  

无 token 时 `sync uex` 明确失败，不写半库。

### 5.5 阶段 A 验收

- sync 后可用 SQL/查询函数列出某 commodity 各终端 buy/sell  
- `status` 显示成功 run 与行数  
- 缺 token 时清晰报错  

## 6. Agent 查询层与 MCP

### 6.1 本地 vs 浏览器

| 类型 | 路径 |
| :--- | :--- |
| 物价、终端、货物快照 | **MCP → SQLite** |
| 任务/蓝图（B 后） | MCP → SQLite |
| 俗称、设定摘要（C 后） | MCP → aliases + FTS |
| Erkul / 采矿站 / Spectrum | 浏览器按需（非本 MCP） |
| Patch Notes | 浏览器；认可结论可手写进 TIPS.md |

第一期**不**自动把浏览器结果写回 SQLite。  
MCP **不**暴露 ingest/sync（写库仍用本地 CLI），避免 Agent 误触发大流量同步；需要时人在终端跑 `sync`。

### 6.2 核心库函数（`sc_search.query`）

与 MCP tool 一一对应，便于单测：

1. `resolve_name(text) -> list[entity]`  
2. `best_prices(commodity, side, system=None, limit=10)`  
3. `terminal_prices(terminal, ...)`  
4. `entity_facts(name_or_id)`  
5. `search_docs(query, limit=5)`  
6. `data_freshness(source=None)`  

回答须带 `game_version` 与最近同步时间；未命中则说明缺失，不编造。

### 6.3 MCP Server

- **传输**：stdio（Cursor / T3 本地 MCP 标准方式）  
- **实现**：`mcp` Python SDK；入口 `python -m sc_search.mcp_server`  
- **库路径**：环境变量 `SC_SEARCH_DB`（默认项目内 `data/sc_knowledge.sqlite`）  
- **Server 名**：`sc-search`（或 `sc_search`）

**第一期（阶段 A）必须注册的 tools：**

| Tool | 作用 |
| :--- | :--- |
| `sc_resolve_name` | 俗称/中英名 → 实体 |
| `sc_best_prices` | 某货物最优买/卖价列表 |
| `sc_terminal_prices` | 某终端报价 |
| `sc_data_freshness` | 各源最近同步时间 / game_version |

**阶段 B/C 再加：** `sc_entity_facts`, `sc_search_docs`（schema 可先占位返回「尚未同步」）。

每个 tool 返回 JSON 文本（含 `ok` / `error` / `data` / `meta.game_version` / `meta.synced_at`），方便 Agent 引用。

### 6.4 项目内注册

在仓库提供 MCP 配置示例（路径按本机绝对路径或 `${workspaceFolder}` 约定）：

```json
{
  "mcpServers": {
    "sc-search": {
      "command": "python",
      "args": ["-m", "sc_search.mcp_server"],
      "cwd": "<SC_Search 根目录>",
      "env": {
        "SC_SEARCH_DB": "<SC_Search>/data/sc_knowledge.sqlite"
      }
    }
  }
}
```

具体落点：优先 `.cursor/mcp.json`（或当前 Cursor/T3 项目 MCP 配置所要求的文件）；并在 `AGENTS.md` 增加一小节：「星际公民事实类问题优先调用 `sc-search` MCP，勿空搜。」

### 6.5 示例路径

用户：「黄金在哪卖？」  
Agent → `sc_resolve_name("黄金")` → `sc_best_prices(commodity="Gold", side="sell")` → `sc_data_freshness(source="uex")` → 结构化回答。

### 6.6 阶段 A MCP 验收

- 本项目 Agent 能列出并调用 `sc-*` tools  
- 库已 sync 时，`sc_best_prices` 返回带终端名的价表与 freshness  
- 库空或未 sync 时返回明确 `error`，不编造价格  

## 7. 非目标（第一期明确不做）

- 向量 embedding / RAG  
- 自动跑商路径规划 UI  
- Erkul/采矿站全量爬取  
- `prices_history`  
- 浏览器结果自动入库  
- 经 MCP 触发 `ingest sync`（写操作保持 CLI）  
- 远程/HTTP MCP（仅本地 stdio）  

## 8. 实现顺序

1. `pyproject.toml` + `sc_search` 包骨架 + schema + `ingest init` + normalize  
2. 阶段 A：UEX adapter + `query`（resolve / prices / freshness）  
3. **MCP server 注册阶段 A 四个 tools + 项目 MCP 配置 + AGENTS.md 指引**  
4. 阶段 B：SCMDB +（稍后）`sc_entity_facts`  
5. 阶段 C：scw_api + citizenwiki + FTS + `sc_search_docs`  

验收门禁：阶段 A 在「有样例数据或真实 UEX sync」下，通用 Agent 仅通过 MCP 即可回答一条物价问题。

## 9. 配置

`config/.env`（gitignore）：

```text
UEX_API_TOKEN=
SC_TRADE_API_TOKEN=   # 可选，后置
SC_SEARCH_DB=         # 可选，覆盖默认 sqlite 路径
```

## 10. 与现有文档的关系

- 检索规则继续遵循根目录 `AGENTS.md`（实现时补充 MCP 优先调用）  
- 站点清单继续维护 `SOURCES.md`（`api.star-citizen.wiki` 在阶段 C 记入）  
- 实战技巧仍沉淀在 `TIPS.md`，与 SQLite 事实库分离  

## 11. 开发与验证工作流

| 角色 | 负责 |
| :--- | :--- |
| **Cursor（本会话）** | 按实现计划写代码、改配置、跑通骨架 |
| **Gemini（委托）** | 测试与验证（单元/CLI/MCP 冒烟）；通过后在 git 中 **commit** |

委托目标固定为：

- **Provider**：`antigravity`（Antigravity）  
- **Model**：`gemini-3.8-flash`（Gemini 3.8 Flash）  
- 需要时在 `target.options` 设 `thought_level`（默认 medium）  

约定：

1. 仓库需先 `git init`（当前工作区尚未是 git 仓库）。  
2. Cursor 完成一个可验证增量后，用 `delegate_task`（antigravity / gemini-3.8-flash）说明改了什么、如何测、期望结果；由其执行测试并 commit（信息聚焦 why）。  
3. Cursor **不**代替该委托做「测试通过后的正式 commit」，除非用户当场改口。  
4. 密钥不进库：`.env`、token、sqlite 大数据文件按 `.gitignore` 排除（schema/小 fixture 可提交）。  


