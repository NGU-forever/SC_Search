# sc_query_plan 路由优化设计

日期：2026-11-21  
状态：待用户审阅  
范围：加速「本地 scw/MCP + 浏览器（Erkul 等）」路径——减少误开浏览器、缩短真需浏览器时的操作步数。  
前置：`2026-11-21-sc-knowledge-sqlite-design.md`（Erkul **不入库**；浏览器按需）。

---

## 1. 背景与问题

Agent 回答《星际公民》问题时，静态事实本可用 `sc-search` MCP（含 `sync scw` 数据）秒回，配装/DPS 才需要 Erkul 浏览器。现状：

- `AGENTS.md` 只写「库未覆盖再开网页」，**没有可执行的意图分流**；
- 第 6 节鼓励优先浏览器查 Erkul，易与「MCP 优先」冲突；
- Erkul 为 SPA，无深链剧本时 Agent 会搜索、乱点、等待，体感很慢。

用户确认痛点为 **两者都有**：识别错 + 浏览器操作慢。

---

## 2. 目标与非目标

### 2.1 目标

- 静态事实（船参数、物价、任务、译名）在 **1～2 次 MCP** 内结束，**默认不开浏览器**。
- 配装 / DPS / 交互计算：MCP 只做「定名 + 给入口」，浏览器 **直达固定 URL、步数有上限**。
- 路由由可测的 `sc_query_plan` 给出，而不是仅靠模型临场猜测。

### 2.2 非目标

- 不爬取、不镜像、不入库 Erkul（及采矿站）数据。
- 不在本 MCP 内实现 DPS / 配装数值计算。
- 不把浏览器结果自动写回 SQLite。
- 不做「为核对而再开 Erkul」的二次浏览。

---

## 3. 意图分流

| 意图 | 典型说法 | `path` |
| :--- | :--- | :--- |
| `facts` | 货舱多大、硬点、厂商、任务奖励 | `mcp_only` |
| `prices` | 哪买/哪卖、终端报价 | `mcp_only` |
| `docs` | lore、机制说明、Wiki 摘要 | `mcp_only` |
| `loadout_calc` | 怎么配、DPS、护盾续航、Erkul | `mcp_then_browser` |
| `mining_calc` | 破矿、精炼模拟 | `mcp_then_browser` |
| `live_web` | Patch、Spectrum、需登录页 | `browser_only` |

`path` 枚举：`mcp_only` | `mcp_then_browser` | `browser_only`。

混合提问（「参数 + 怎么配」）归为 `loadout_calc`：仍先跑 MCP facts，并用 `stop_if` 标明——若用户只要参数则不得开浏览器。

---

## 4. `sc_query_plan` API

### 4.1 工具

- **名**：`sc_query_plan`
- **实现**：`sc_search.query.plan`（纯函数，可单测）+ `sc_search.mcp_server` 注册
- **返回包装**：与现有 tools 一致 `{ ok, data, meta, error }`

### 4.2 入参

| 参数 | 类型 | 说明 |
| :--- | :--- | :--- |
| `question` | string | 用户原话（中英均可） |
| `hint_entity` | string \| 省略 | 可选；已知船/货名，降低歧义 |

### 4.3 `data` 字段

| 字段 | 说明 |
| :--- | :--- |
| `intent` | 上表意图枚举 |
| `path` | `mcp_only` / `mcp_then_browser` / `browser_only` |
| `confidence` | `high` \| `medium` \| `low` |
| `entities` | 解析到的实体摘要（canonical 英文名、kind、id、可选中文） |
| `mcp_steps` | 建议按序调用的 MCP tool + args（可为空列表） |
| `browser` | 见下；`needed=false` 时其余字段可省略或为空 |
| `stop_if` | 人读短句：何时截断、不要开浏览器 |
| `reason` | 一句话说明为何选此 path |

### 4.4 `browser` 对象（`needed=true` 时）

| 字段 | 说明 |
| :--- | :--- |
| `needed` | bool |
| `site` | 如 `erkul` / `regolith` |
| `start_url` | 直达入口：Erkul 默认 `https://erkul.games/calculator`；采矿默认 `https://regolith.rocks`（见 SOURCES） |
| `search_term` | **英文标准名**（来自 resolve，禁止用未解析俗称） |
| `max_actions` | 默认 `8` |
| `extract` | 应提取的关键信息列表 |
| `do_not` | 禁止行为（含 `/auth`、`/settings`、先搜其它站等） |

### 4.5 分类规则（可测、少魔法）

按优先级：

1. 关键词 → `loadout_calc`：`dps`、`配装`、`loadout`、`erkul`、`怎么配`、`打法`（配装语境）等  
2. 关键词 → `mining_calc`：`破矿`、`精炼模拟`、`regolith`、`laser head` 配矿等  
3. 关键词 → `prices`：`买`/`卖`/`价格`/`哪出`/`报价` 等（货物语境）  
4. 关键词 → `live_web`：`patch notes`、`spectrum`、需登录官网等  
5. 关键词 → `docs`：明显 lore/设定/wiki 摘要类  
6. 否则若能 resolve 到实体 → `facts`（**默认不开浏览器**）  
7. 无法判断 → `confidence=low`，`path=mcp_only` 先 `sc_resolve_name`，`stop_if` 提示勿盲目开浏览器  

`hint_entity` 存在时：先 resolve hint，再填 `entities` / `search_term`。

### 4.6 示例返回（缩略）

用户：「卡拉克怎么配打炮」

```json
{
  "ok": true,
  "data": {
    "intent": "loadout_calc",
    "path": "mcp_then_browser",
    "confidence": "high",
    "entities": [{"canonical_name": "Carrack", "kind": "ship", "zh": "卡拉克"}],
    "mcp_steps": [
      {"tool": "sc_resolve_name", "args": {"text": "卡拉克"}},
      {"tool": "sc_entity_facts", "args": {"name_or_id": "Carrack"}}
    ],
    "browser": {
      "needed": true,
      "site": "erkul",
      "start_url": "https://erkul.games/calculator",
      "search_term": "Carrack",
      "max_actions": 8,
      "extract": ["selected ship", "DPS summary", "shield / power notes"],
      "do_not": ["open /auth", "/settings", "wander other sites first"]
    },
    "stop_if": "若用户只要静态参数（货舱/硬点），答完 facts 即停，不开浏览器",
    "reason": "配装/DPS 需 Erkul；库无实时计算"
  },
  "meta": {}
}
```

---

## 5. Agent 规则与浏览器剧本

修改 `AGENTS.md`（收紧 MCP 段 + 第 6 节）：

1. **强制顺序**：`sc_query_plan` → 执行 `mcp_steps` → 仅当 `browser.needed=true` 开浏览器。禁止先 WebSearch / 先开 Erkul 再查库。  
2. **`path=mcp_only`**：答完即停；禁止「再打开 Erkul 核对」。缺字段如实说明，可问用户是否要上 Erkul。  
3. **`loadout_calc` 浏览器剧本**：只用 plan 的 `start_url`；`search_term` 必须为英文标准名；遵守 `max_actions`（打开→搜船→读摘要→取数→关标签）；禁止 `/auth`、`/settings`；同一题不另开第二站，除非 plan 指定。  
4. **失败降级**：页面超时/失败时，用已有 scw facts 答静态部分，并标明「DPS/配装未取到」。  
5. **第 6 节措辞**：浏览器仅在 plan 要求时使用；不以 ego-browser 作为日常事实查询的优先路径。

Erkul 入口以 sitemap 已公开路径为准：`/calculator`、`/ships`、`/finder`、`/browse`、`/data`；配装默认 `/calculator`。遵守该站 `robots.txt`：禁止 `/auth/`、`/settings`；Content-Signal 含 `ai-train=no`，本方案仅 **reference 按需读取**，不做训练语料采集。

---

## 6. 测试与验收

### 6.1 单测（`query.plan`）

| 用例问题 | 期望 intent / path | 浏览器 |
| :--- | :--- | :--- |
| 卡拉克货舱多大 | `facts` / `mcp_only` | `needed=false` |
| 黄金在哪卖 | `prices` / `mcp_only` | `needed=false` |
| 卡拉克怎么配打炮 | `loadout_calc` / `mcp_then_browser` | Erkul + `search_term=Carrack` |
| Carrack best DPS loadout | `loadout_calc` / `mcp_then_browser` | 同上 |
| 鼹鼠怎么配激光头 | `mining_calc` / `mcp_then_browser` | 采矿站入口 |

### 6.2 人工 / Agent 行为验收

- 「卡拉克货舱多大」→ 调用链无浏览器工具。  
- 「卡拉克怎么配打炮」→ 先 plan + facts，再 Erkul，动作 ≤ `max_actions`。  
- 不得出现：未 plan 就打开 Erkul；或用中文俗称当 Erkul 搜索词（未 resolve 时）。

---

## 7. 实现范围（落地时）

1. 新增 `src/sc_search/query/plan.py` + 单测。  
2. MCP 注册 `sc_query_plan`。  
3. 更新 `AGENTS.md` 路由与浏览器小节。  
4. 可选：`README.md` 工具列表补一行。  
5. **不**改 ingest / 不新增 Erkul adapter。

---

## 8. 与既有设计的关系

- 延续「Erkul / 采矿站不入库、浏览器按需」。  
- 本设计补的是 **按需的识别与执行纪律**，不是新数据源。  
- 若未来扩展 scw 字段（减少浏览器），属独立增量，不在本文范围。
