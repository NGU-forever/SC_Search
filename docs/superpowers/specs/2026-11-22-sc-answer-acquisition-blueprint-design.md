# sc_answer P0：蓝图实体买/价问法的 acquisition 证据

日期：2026-11-22  
状态：待用户审阅  
范围：仅增强 `sc_answer`（`answer.py` + 测试；可选 `AGENTS.md` 一句）——买/价问法命中 `kind=blueprint` 时早退，输出结构化 `evidence.acquisition` 与防旧常识草稿。  
前置：`2026-11-21-sc-answer-oneshot-design.md`、`2026-11-21-sc-answer-p0-confidence-zh-design.md`  
动机：JS-400 类案例中本地库已有 `blueprint` + `pool_name`，但草稿写成「未找到匹配字段」，Agent 易被旧 wiki 商店价带偏；同时「视差」等武器存在「可买可造」双通道，不能把 blueprint 一律断言为「商店不可买」。

---

## 1. 目标与非目标

### 1.1 目标

1. **买/价问法 + 主实体 `kind=blueprint`**：在 facts/prices 空草稿之前**早退**，写入 `evidence.acquisition`。  
2. **`shop_buyable` 三态**：P0 默认走 `"unknown"`（有蓝图/奖励线索、无商店报价证据）；预留 `"false"` / `"true"`，本期不实现下架标记源。  
3. **草稿防污染**：必须点明蓝图/奖励线索与「未找到商店报价」；**禁止**编造店名/历史标价；**禁止**在 `unknown` 时写死「商店不可直接购买」。  
4. **置信度**：本路径顶层 `confidence=high`（对「本地商店证据有无」为高置信）；`browser.needed=false`；不追加「要用浏览器核实吗？」。

### 1.2 非目标

- 不改 `query_plan` 意图正则（含「哪里买」是否进 `prices`）。  
- 不改 ingest；不持久化 `acquisition_mode` / `shop_buyable` fact。  
- 不反查 mission 可读名；`pool` 仅用原始 `pool_name`。  
- 不描述品质差（CQ 无品质 vs 制造高品质）——无结构化数据不写。  
- 不实现「明确下架 seed」使 `shop_buyable=false`（字段与文案规则预留即可）。  
- 不新增 MCP 工具；不改 `sc_answer` 入参签名。

---

## 2. 触发与数据流

**触发（需同时满足）**

1. 问题匹配买/价意图（实现时用小正则，至少覆盖：买、哪里买、能买到、价格、报价、`price`、`buy` 等；与 `plan.py` 可略有重复，P0 可接受）。  
2. `query_plan` / resolve 得到的主实体 `kind == "blueprint"`。

**不触发**

- 非买/价问法（如「JS-400 是什么」）→ 现有 facts。  
- `kind ≠ blueprint`（如 commodity 黄金）→ 现有 prices/facts。

**数据流**

```
question → query_plan（照旧）
        → primary entity
        → 若 买/价 ∧ kind=blueprint
             → entity_facts（取 pool_name 等）
             → evidence.acquisition + 中文 draft
             → confidence=high, browser.needed=false
             → return（跳过「未找到匹配字段」与空报价「去某店」暗示）
        → 否则现有 facts / prices / …
```

`intent` / `path` / 嵌套 `plan.*`：保留 plan 原值，本 P0 不改路由字段。

**实现形态**：方案 1 — 在 `answer.py` 内联早退（推断与草稿可为同文件私有函数；不强制新模块）。

---

## 3. `evidence.acquisition` 契约

固定挂在 **`evidence.acquisition`**（不新增 MCP 顶层字段）。

```json
{
  "mode": "blueprint_reward",
  "shop_buyable": "unknown",
  "pool": "BP_REWARDS_FullStrikeOnStationB",
  "hint_zh": "本地仅见蓝图/奖励记录，未找到商店报价"
}
```

| 字段 | 规则 |
| :--- | :--- |
| `mode` | 本期固定 `"blueprint_reward"` |
| `shop_buyable` | `"unknown"` \| `"false"` \| `"true"`（字符串枚举，便于 JSON/Agent） |
| `pool` | fact `pool_name` 的 `value_text`；缺失则为 `null` |
| `hint_zh` | 与当前 `shop_buyable` 状态匹配的短句，供 Agent 引用 |

### 3.1 `shop_buyable` 语义

| 值 | 何时（P0） | 草稿要点 | confidence |
| :--- | :--- | :--- | :--- |
| `"unknown"` | **默认**：blueprint + 买/价 + 无商店报价证据 | 蓝图/奖励（有则写池名）+「未找到商店报价」；**不**断言绝对买不到 | `high` |
| `"false"` | 仅当存在明确下架/仅奖励标记（**本期无标记源，不实现判定**） | 「商店不可直接购买」 | `high` |
| `"true"` | 存在商店报价证据（可选；非 P0 必做分支） | 可走报价逻辑；可附带「另有蓝图通道」 | 沿用 prices 或本路径约定 |

P0 验收以 **`unknown`** 为主路径（含 JS-400）。

### 3.2 草稿模板（`unknown`）

有池名时示例：

> JS-400 本地仅见蓝图/任务奖励记录（池：BP_REWARDS_FullStrikeOnStationB），未找到商店报价；不能据此断言绝对买不到（Live 4.10.3）。

无 `pool_name`：省略「（池：…）」，仍须含「蓝图/奖励」类表述 +「未找到商店报价」类表述。  
Live 后缀沿用 `_draft_live_suffix`。

**禁止**（`unknown` 草稿与 Agent 成文）

- 写死「商店不可直接购买」  
- 出现未在 evidence 中的店名、历史 aUEC 标价  

### 3.3 其它返回字段

- `browser.needed`: `false`  
- 不追加 `_SOFT_VERIFY`（「要用浏览器核实吗？」）  
- `evidence.missing`：本路径不放入 `matching_facts` / `prices`（避免像「库不够」）  
- `evidence.entities` / 可选精简 facts：允许带上已读到的 `pool_name` 等，便于调试；非强制

---

## 4. 与双通道（视差等）的关系

「视差」类：终端可买（常无品质）+ 蓝图可造（材料影响品质）。  
P0 **不**建模品质；用 `"unknown"` 避免把双通道误杀成不可买。  
后续若有商店价表或下架 seed，再分别落到 `"true"` / `"false"`，并可另开「品质差」说明规格。

---

## 5. Agent / AGENTS.md（可选一句）

买/价问法若 evidence 含 `acquisition`：成文必须遵循 `shop_buyable`；`unknown` 时说明无商店证据与蓝图线索，禁止用 Web/旧 wiki 店价覆盖 MCP 证据。

---

## 6. 测试验收

文件：`tests/test_answer.py`（fixture 内 upsert blueprint + `pool_name`）。

| 问 | 期望 |
| :--- | :--- |
| JS-400 哪里买 / 多少钱 / 能买到吗 | `acquisition.shop_buyable == "unknown"`；draft 含蓝图或奖励、含未找到商店报价；**不含**「商店不可直接购买」；`confidence=high`；`browser.needed=false` |
| JS-400 是什么 | **不**强制本早退（无强制 `acquisition`） |
| 黄金哪卖（有 UEX 价） | 行为不变 |

---

## 7. 实现范围清单

1. `src/sc_search/query/answer.py`：买/价检测、blueprint 早退、`evidence.acquisition`、draft、confidence/browser。  
2. `tests/test_answer.py`：上表用例。  
3. 可选：`AGENTS.md` 一句 acquisition 成文规则。

---

## 8. 与既有设计关系

本 P0 是 oneshot / 置信度 P0 的**获取通道补丁**：不改变「默认 sc_answer、浏览器门禁、Live 以 SCMDB 为准」。  
不替代后续：intent 路由修复、ingest `acquisition_mode`、下架 seed、品质说明。
