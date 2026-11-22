# SC Search — 问答编排（ORCHESTRATION）

由 [AGENTS.md](AGENTS.md) 引用的完整流水线。事实数据优先走本地 MCP **`sc-search`**；浏览器只作确认后的补充，不是默认第一步。

机器可读检索契约：[`src/sc_search/query/intent_tools.py`](src/sc_search/query/intent_tools.py) 的 `INTENT_TOOLS`（与下文剧本表保持一致）。

---

## 总编排（每题必走）

```text
用户提问
   │
   ▼
① 理解  定 primary + 槽位；纠正俗称；禁止先开浏览器 / 泛化 WebSearch
   │
   ▼
② 检索  只调 sc-search 白名单；按「意图 → 工具」顺序取证（已有证据可跳过）
   │
   ▼
③ 成文  证据包只是脚手架 → LLM 深度整理成玩家可读中文；库无字段不编造
   │
   ▼
④ 浏览器（可选）  仅当 path 需要且用户确认；交互站 leave_open 交给用户继续改
```

| 阶段 | 做什么 | 不做什么 |
| :--- | :--- | :--- |
| ① 理解 | 判 `primary`、填槽位；俗称归一（如红枫物流→红风物流 / Red Wind Linehaul） | 先 WebSearch、先 Erkul |
| ② 检索 | 白名单工具按序调用；单实体静态度/价可直接 `sc_answer` | 空搜、编造库外数字 |
| ③ 成文 | 归纳结论、对照字段、版本/来源、操作含义；可交叉多条 evidence | 原样甩 JSON、只贴链接 |
| ④ 浏览器 | 见「浏览器子编排」 | 未确认就开；`mcp_only`+高置信仍开 |

**默认入口**：单实体静态参数/物价 → 直接 `sc_answer`。口语复杂（声望、多约束任务、对比）→ 先定 `primary` 再跑剧本（不必先 `sc_query_plan`；该工具仅路由/调试可选）。

---

## ① 理解 — 意图与槽位

从用户话里选出一个 `primary`（见下表），并尽量抽出槽位。短码（如 `cq7`）整句未命中时，先 `sc_resolve_name(短码)`；蓝图/制造语境优先 `kind=blueprint` 基型，勿被皮肤名带偏。俗称 seed 例：`视差`→Parallax Energy Assault Rifle；`卡拉克`→Carrack。

中文外搜前：先用汉化组/Wiki 标准译名对照，禁止自行乱译去搜。外搜 query 必须含 `星际公民` / `Star Citizen` / `SC`。优先 [SOURCES.md](SOURCES.md) 中的站点。

---

## ② 检索 — 意图剧本（编排表）

| primary | 槽位 | 工具顺序 | path |
| :--- | :--- | :--- | :--- |
| `trade` | commodity, side, system, terminal | `sc_answer` → `sc_best_prices` → `sc_terminal_prices` | mcp_only |
| `ship_stats` | ship | `sc_answer` → `sc_resolve_name` → `sc_entity_facts` | mcp_only |
| `ship_buy` | ship | `sc_answer` → resolve → facts →（可选）`sc_search_docs`；购点外链确认后再开浏览器 | mcp_then_browser |
| `mission_rep` | faction, rank, system | `sc_resolve_name`(势力) → `sc_list_missions` → `sc_entity_facts`；**不编造声望点数** | mcp_only |
| `mission_money` | activity, system, faction | `sc_list_missions` → resolve →（可选）`sc_answer` | mcp_only |
| `blueprint` / `crafting` | item, blueprint | `sc_answer` → resolve（偏 blueprint）→ `sc_entity_facts`；矿点未收录则明说 | mcp_only |
| `loadout` | ship | 静态：`sc_answer` / resolve / `sc_entity_facts`；要 DPS/配装结果再 Erkul | mcp_then_browser |
| `mining` | ship, laser | 静态同上；确认后再开 mining guides | mcp_then_browser |
| `bug_tip` / `client_help` | topic | `sc_search_docs`（+ 可读本地 tips）；不足再外链，确认后打开 | mcp_then_browser |
| `howto` | topic | `sc_search_docs` / `sc_answer` + howto guides | mcp_then_browser |
| `earn_meta` | activity, system | `sc_list_missions` → `sc_answer` / docs；禁止编造时薪 | mcp_then_browser |
| `pledge_meta` | topic | docs + pledge guides（库无账单） | mcp_then_browser |
| `event_guide` | event | docs / `sc_data_freshness`；时效活动确认后开外链 | mcp_then_browser |
| `lore_docs` | query | `sc_search_docs` →（可选）`sc_answer` | mcp_only |
| `trade_route` | commodity, system | `sc_best_prices` / `sc_answer`；整条航线确认后开规划站 | mcp_then_browser |
| `patch_live` | — | `sc_data_freshness` 对照库版本；正文以官网为准 | browser_only |
| `other` | — | `sc_answer` → resolve →（可选）`sc_query_plan`；勿空搜 | mcp_only |

**白名单工具**：`sc_answer`、`sc_query_plan`、`sc_resolve_name`、`sc_best_prices`、`sc_terminal_prices`、`sc_data_freshness`、`sc_entity_facts`、`sc_list_missions`、`sc_search_docs`。

**写入/同步**（MCP 只读问答侧不触发）：`sync uex`｜`sync scmdb [--channel live|ptu]`｜`sync scw`｜`sync scw_blueprints`｜`sync wiki`｜`sync aliases`｜`sync zh`。

**版本**：Live 以 SCMDB sync 为准（当前 **4.10.3**）。短答用 `live_game_version` / 「Live 4.10.3」；勿把 UEX 自带旧 `game_version` 当成当前 Live。

浏览器 `search_term` 必须用 plan / `sc_answer` 证据里的**英文标准名**。

---

## ③ 成文 — 证据 → 玩家答案

`answer_draft` / `evidence` / `reasoning_hint` / 外链 = **脚手架**，不是终稿。

1. 证据非空 → 深度整理：结论、字段对照、分点或小表、版本与来源、实操注意。
2. 数字、店名、任务名、材料等**只能来自证据**；缺则写「本地未收录」。
3. 禁止编造：库外店价、掉率、刷新点、时薪、矿点。
4. **获取通道**：若有 `evidence.acquisition`，成文遵循 `shop_buyable`；`unknown` 时说明无商店证据，有 `missions`/`materials` 则引用；`missing` 含配方/矿点未收录时明说。勿用证据外旧店名覆盖。
5. 输出偏表格、关键数据、步骤；提示 Armistice、PvP、易卡 Bug 等风险。

---

## ④ 浏览器子编排

```text
path / confidence 门禁
   │
   ├─ mcp_only 且 confidence=high     → 禁止开浏览器
   ├─ confidence=low（或无命中 medium）→ 勿擅自开；follow_ups 问核实则等确认
   └─ mcp_then_browser / browser_only → 先成文 + 列 resources；等用户确认
            │
            ▼
      打开 start_url（search_term 英文标准名）
            │
            ▼
      ≤ max_actions：搜索/定位 → 提取 extract
            │
            ├─ leave_open=true（Erkul / 采矿计算站等）
            │     → 摘要后不要关页；面板留在会话里给用户继续改
            └─ 其它一次性页
                  → 可关标签
```

| 规则 | 说明 |
| :--- | :--- |
| 门禁 | `wait_confirm`：要精确计算 / 打开教程 / 用户明确核对才开 |
| 剧本 | 只用返回的 `start_url` 或用户点名的 `resources[].url` |
| 交接 | `leave_open` / `handoff_to_user`：答完禁止主动关该标签；用户说关或换站再关 |
| 禁止 | `/auth`、`/settings`；未写明另开第二站；中文俗称当 Erkul 搜索词；把搜索页内容编成库内数值 |
| 降级 | 超时/失败 → 用已有 MCP 证据答静态部分，标明未取到；若已开且 `leave_open` 仍留给用户 |
| 宿主 | 优先会话内可展示给用户的预览浏览器，否则 `ego-browser` |

---

## 旁路协议（非问答主链，仍要遵守）

### 外搜与中文

- 外搜必须带 SC 上下文；禁止泛化搜索。
- 中英术语可混用；中文站检索前先完成标准译名对照。

### 站点沉淀

用户分享的任何站点/工具链接 → **立即**追加到 [SOURCES.md](SOURCES.md)（名称、URL、用途、日期、说明），后续优先复用。

### 领域默认

提问默认指星际公民，无需反复确认。机制/配装建议注明 Live / PTU 等版本环境。

### 术语

熟练使用 aUEC、SCU、Stanton/Pyro、QT、Master Modes、Soft Death、各厂家与常见俗称（大白鲨、切口、奥里森、微凸等）。
