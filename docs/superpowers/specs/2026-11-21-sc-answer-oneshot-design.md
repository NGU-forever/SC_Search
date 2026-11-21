# sc_answer 一键作答设计

日期：2026-11-21  
状态：待用户审阅  
范围：一次 MCP 调用完成「归类 → 查库 → 短答草稿 + 证据」；详细内容按需展开；浏览器仅低置信 / 明确核对 / 配装类计算时建议。  
前置：

- `2026-11-21-sc-knowledge-sqlite-design.md`
- `2026-11-21-sc-query-plan-routing-design.md`（`query_plan` / 浏览器门禁）

---

## 1. 背景与问题

`sc_query_plan` 已能分流 MCP vs 浏览器，但 Agent 仍常：`plan` → `resolve` → `facts`/`prices` 多轮往返，体感慢。用户要：

1. **索引式归类后一次取数**；
2. **短答 + 证据**；详细等追问再给；
3. 仅确认 / 不确定 / 配装计算时再上浏览器。

选定方案：**新工具 `sc_answer`**（包现有 `query_plan` + 本地 query），带 `detail_level=brief|full`；保留 `sc_query_plan` 供调试。

---

## 2. 目标与非目标

### 2.1 目标

- 默认一次 `sc_answer(question)` 得到可转发的中文短答 + 结构化证据。
- `brief` 先答；用户要详细或 `detail_level=full` 再加料。
- 高置信本地事实：**禁止**为核对开浏览器。
- 配装/采矿/官网类：先给本地能答的部分，浏览器按 plan 字段**建议**，不默认连开。

### 2.2 非目标

- 工具内不用 LLM 写长文（模板拼 `answer_draft`）。
- 不爬 Erkul / 采矿站，不写回 SQLite。
- 不在 MCP 内替代 DPS/配装实时计算。
- 不删除 `sc_query_plan`（降为可选）。

---

## 3. 主流程

```
用户问题
  → sc_answer(question, detail_level=brief|full, hint_entity?)
      → query_plan(...)
      → 按 intent 执行本地查询
      → 拼 answer_draft + evidence + follow_ups + browser
      → Agent：优先转发短答；仅门禁允许时开浏览器
```

| 工具 | 角色 |
| :--- | :--- |
| `sc_answer` | Agent **默认入口** |
| `sc_query_plan` | 只要路由、调试 |
| `sc_entity_facts` 等 | 追问时可由 `sc_answer(full)` 内部多取，或 Agent 直调 |

---

## 4. API

### 4.1 入参

| 参数 | 类型 | 默认 | 说明 |
| :--- | :--- | :--- | :--- |
| `question` | string | 必填 | 用户原话 |
| `detail_level` | `"brief"` \| `"full"` | `"brief"` | 证据厚度 |
| `hint_entity` | string \| 省略 | — | 已知实体 |

### 4.2 返回

沿用 `{ ok, data, meta, error }`。`data`：

| 字段 | 说明 |
| :--- | :--- |
| `intent` / `path` / `confidence` | 来自 `query_plan` |
| `answer_draft` | 短中文，可直接给用户 |
| `evidence` | 精简结构（实体、facts/prices/docs 摘录） |
| `detail_available` | 是否还有可展开内容 |
| `follow_ups` | 建议追问文案列表 |
| `browser` | 同 `query_plan.browser` |
| `plan` | 嵌套完整 plan（便于调试） |

`meta`：尽量带 `game_version`、相关源的同步时间（有则填）。

### 4.3 brief vs full（示例）

**brief**「卡拉克货舱多大」  
→ 一句话结论 + 版本；evidence 1～3 条相关 fact；`detail_available=true`；提示可展开。

**full**  
→ 同结论 + evidence 含更多 facts/relations（仍设上限，不做整库 dump）。

---

## 5. 浏览器门禁

1. `path=mcp_only` 且 `confidence=high` → **禁止**开浏览器，只用 `answer_draft`。  
2. `loadout_calc` / `mining_calc` → 先答静态；草稿标明需 Erkul/Regolith；**仅当用户要配装结果或明确核对**再按 `browser` 开。  
3. `confidence=low` 或未命中 → 说明缺口；浏览器为可选下一步，**不默认连开**。  
4. `live_web` → `browser.needed=true`，草稿说明需官网/论坛。

禁止 `/auth`、`/settings`；`search_term` 仅英文标准名（空则不开搜）。

---

## 6. 按 intent 取数

| intent | brief | full 额外 | draft 要点 |
| :--- | :--- | :--- | :--- |
| `facts` | `entity_facts`，相关 key 上限约 5 | 更多 facts + 重要 relations | 结论 + 版本 |
| `prices` | `best_prices` top 3 | top 10；问到终端则加 `terminal_prices` | 最优点 + 价 |
| `docs` | `search_docs` 1～2 条摘录 | 最多 5 | 短摘要 |
| `loadout_calc` / `mining_calc` | 有船则静态 facts | 同 facts full | 静态 +「配装需工具站」 |
| `live_web` | 几乎无本地 | — | 需浏览器 |
| 未命中 | 空 evidence | — | 库未覆盖；不编造；`detail_available=false` |

模板：中文短句；有则写 `game_version`/源；**禁止捏造数值**。

`evidence` 建议形状（实现可微调，单测锁关键字段）：

```json
{
  "entities": [{"canonical_name": "Carrack", "kind": "ship", "zh": "卡拉克"}],
  "facts": [{"key": "cargo", "value_text": "...", "unit": "SCU", "source": "scw"}],
  "prices": [],
  "docs": [],
  "missing": []
}
```

---

## 7. AGENTS.md

- 事实类默认：`sc_answer(question)`（默认 brief）。  
- 「详细 / 展开 / 全部…」→ `detail_level="full"` 或 `sc_entity_facts`。  
- `sc_query_plan` 可选（调试）。  
- 浏览器服从第 5 节；不为核对高置信本地答案开 Erkul。

---

## 8. 实现范围

1. `src/sc_search/query/answer.py` — `answer_question(conn, question, detail_level=..., hint_entity=...)`  
2. MCP 注册 `sc_answer`；`handlers` 导出  
3. `tests/test_answer.py` + MCP smoke  
4. 更新 `AGENTS.md`、`README.md`  
5. **不**改 ingest；**不**新增 Erkul adapter  

---

## 9. 验收

| 场景 | 期望 |
| :--- | :--- |
| 「卡拉克货舱多大」 | 一次 `sc_answer`；有 draft；`browser.needed=false` |
| 再「详细说说」 | full / 二次调用；evidence 更厚 |
| 「黄金在哪卖」 | draft 含终端+价；mcp_only |
| 「卡拉克怎么配」 | 静态摘要 + 需 Erkul 提示；用户未要求时不开浏览器 |
| 未知名 | 明确缺数据；不编造 |

单测覆盖：facts brief、prices、loadout 静态+browser 标志、未命中、`detail_level` 厚度差异。

---

## 10. 与既有设计关系

- 复用 `query_plan` 意图与浏览器对象，不重复爬虫策略。  
- 解决的是 **多轮 MCP 延迟与「先短后详」**，不是新数据源。
