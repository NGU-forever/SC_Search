# sc_answer P0：证据置信度 + 中文 key 映射

日期：2026-11-21  
状态：待用户审阅  
范围：仅增强 `sc_answer` 后处理——按证据重算 `confidence`、draft 中文 fact 标签、`low` 时软建议浏览器（不自动打开）。  
前置：`2026-11-21-sc-answer-oneshot-design.md`

---

## 1. 目标与非目标

### 1.1 目标

1. **证据驱动置信度**：取数后根据是否真有匹配 fact/报价/文档，重算顶层 `confidence`（以 answer 为准）。  
2. **中文草稿**：高频 fact key 用静态中文标签展示（如货舱、制造商）。  
3. **low 软建议**：对 facts/prices/docs，`confidence=low` 时 `browser.needed=false`，`follow_ups` 追加「要用浏览器核实吗？」；不擅自开浏览器。

### 1.2 非目标

- 不改 `query_plan` 意图正则 / 多意图。  
- 不把 `browser.needed` 因 low 自动改为 true。  
- 不从 localization ini 动态翻译任意 key。  
- 不改 ingest / Live 版本权威逻辑（仍 SCMDB → Live 4.10.3）。

---

## 2. 置信度规则

取数完成后覆盖顶层 `confidence`。嵌套 `plan.confidence` **保留 plan 原值**（调试可对比）。

| 场景 | confidence |
| :--- | :--- |
| facts：问题相关字段有有效 value | `high` |
| facts：有实体但无匹配字段 / 仅泛化列表 | `medium` |
| facts：无实体或关键 `missing` | `low` |
| prices：≥1 条报价 | `high` |
| prices：无报价 | `low` |
| docs：有命中 | `medium` |
| docs：无命中 | `low` |
| loadout/mining：有静态 facts | `medium` |
| loadout/mining：无静态 | `low` |
| live_web | 沿用 plan（通常需站外） |

### 2.1 与浏览器

| intent | `confidence=low` 时 |
| :--- | :--- |
| facts / prices / docs | 强制 `browser.needed=false`；`follow_ups` 含「要用浏览器核实吗？」 |
| loadout_calc / mining_calc | **不**因本 P0 改写 `needed`（仍「用户要结果才开」） |
| live_web | 仍跟 plan |

Agent 规则：以顶层 `confidence` 为准；low 时除非用户回应核实，否则不开浏览器。

---

## 3. 中文 key 映射

静态表 `_FACT_KEY_ZH`（可随常用字段扩展），至少：

| key（含常见别名） | 中文 |
| :--- | :--- |
| `cargo_capacity`, `cargo_scu`, `cargo` | 货舱 |
| `manufacturer` | 制造商 |
| `manufacturer_code` | 制造商代码 |
| `length` | 长度 |
| `beam` | 宽度 |
| `height` | 高度 |
| `mass`, `mass_total` | 质量 |
| `size` | 尺寸 |
| `type` / `career` / `foci` | 类型 / 职业 / 定位 |
| `production_status` | 生产状态 |
| `crew` | 乘员 |
| `hp` | 结构值 |

未知 key：draft 中保留英文 key。  
`evidence.facts[].key` 仍存原始英文（便于程序）；可选同时写 `key_zh`（推荐，便于 Agent）。

draft 示例：`卡拉克（Carrack）的货舱为 456 SCU（Live 4.10.3）。`

---

## 4. `detail_available`

仅当确实还有未展示内容时为 `true`：

- facts：`len(all_facts) > len(picked)` 或 full 下还有未纳入的 relations；  
- prices：库内明显还有更多终端未展示；  
- brief 且已展示全部可展示项 → `false`（修正「brief 恒 true」）。

---

## 5. 实现范围

1. `src/sc_search/query/answer.py`：映射表、`_evidence_confidence`、中文 draft、`detail_available` 修正、low follow_up。  
2. `tests/test_answer.py`：验收例。  
3. `AGENTS.md`：一句「以 sc_answer.confidence 为准；low 勿擅自开浏览器」。  
4. 不改 MCP 签名（仍 `sc_answer(question, detail_level, hint_entity)`）。

---

## 6. 验收

| 问 | 期望 |
| :--- | :--- |
| 卡拉克货舱多大 | draft 含「货舱」；`confidence=high`；`browser.needed=false` |
| 有实体但问不存在字段 | `medium` 或 `low`；核实 follow_up；`needed=false` |
| 黄金在哪卖（有价） | `high`；Live 标签仍在 |
| 无报价商品 | `low`；「要用浏览器核实吗？」；`needed=false` |
| 卡拉克怎么配 | 仍可为 `browser.needed=true`（intent）；置信度 medium/low 按静态有无 |

---

## 7. 与既有设计关系

本 P0 是 oneshot 设计的**质量补丁**，不改变「默认 sc_answer、浏览器门禁、Live 以 SCMDB 为准」的主流程。
