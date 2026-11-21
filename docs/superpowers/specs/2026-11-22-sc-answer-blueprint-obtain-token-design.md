# sc_answer 一期：短码 Token 解析 + 蓝图获取草稿编译

日期：2026-11-22  
状态：已按 grill-me 默认拍板（用户授权「走默认」长任务）  
范围：`query_plan` / `resolve` 问句短码命中；`sc_answer` 扩展 obtain 触发；blueprint 获取草稿含 pool + mission；材料缺数据结构化 missing。  
前置：`2026-11-22-sc-answer-acquisition-blueprint-design.md`（买/价 `unknown` 路径保留）  
动机：CQ7 整句 `sc_answer` 报「未覆盖」；hint 后仍「未找到匹配字段」；「获取/材料」不进 acquisition；库无配方。

---

## 1. Grill-me 默认决策

| 题 | 默认 |
| :--- | :--- |
| 范围 | 仅一期 query 层（不做 recipe ingest、不做品质双通道） |
| 短码策略 | Token 抽词 + 现有 `resolve_name(token)`；不做全库短别名 seed |
| 同名冲突 | 问句含蓝图/制造/获取/配方/材料时，primary **优先 `blueprint`** |
| obtain 触发 | 买/价 **或** 获取/怎么得/掉落/奖励/蓝图/制造/制作/配方/材料/crafting |
| mission | **要** 反查 `mission_reward_blueprint_pool` → 中文名/承包商/星系写入 draft + acquisition |
| shop_buyable | 仍默认 `"unknown"`（与既有 P0 一致） |
| 材料 | `missing` 含 `crafting_recipe`；草稿诚实缺数据；不软核实去编材料 |
| 协作 | Cursor 开发；Antigravity `gemini-3.8-flash` 跑测 + commit |

---

## 2. 目标与非目标

### 2.1 目标

1. `cq7的蓝图怎么获取…` → 命中 `CQ7 Rifle` blueprint，不再「实体未覆盖」。  
2. obtain/买价 + blueprint → `evidence.acquisition` + 草稿含池名；有则含任务可读线索。  
3. 问到材料且无配方 → 明确「库无制造材料/配方」，不编造。  
4. 既有买/价 `unknown` 行为与黄金物价用例不回归。

### 2.2 非目标

- 不 ingest 制造配方。  
- 不改 MCP 工具签名。  
- 不做视差品质差文案。  
- 不默认开浏览器补材料。

---

## 3. Slice A — Token 解析 + blueprint 优先

**改动**：`plan.py`（及必要时小幅 `resolve` 辅助）。

1. 从问句提取 token（至少：ASCII 词 `[A-Za-z][A-Za-z0-9\-]*`，长度≥2；保留大小写不敏感匹配）。  
2. 对每个 token 调用 `resolve_name(conn, token)`，合并进 `_resolve_entities`（去重）。  
3. 若问句匹配蓝图语境正则（蓝图|制造|制作|配方|材料|获取|crafting|blueprint），对 entities 排序时 `blueprint` 优于同前缀 item 皮肤。  
4. 长 alias embedded 命中仍保留（更长更优）。

**验收**：

- `query_plan("cq7的蓝图怎么获取以及我要制造的话，材料怎么得。")` → entities 含 `kind=blueprint` `CQ7 Rifle`（可为 primary）。  
- `answer_question` 同句不再 `missing: ["entity"]`。

---

## 4. Slice B — obtain 扩展 + pool→mission 草稿

**改动**：`answer.py`（可加同文件私有函数）。

1. 扩展 `_OBTAIN_RE`（或并列 obtain 正则）覆盖 §1 触发词。  
2. Blueprint + obtain：  
   - `acquisition.mode = blueprint_reward`  
   - `shop_buyable = "unknown"`  
   - `pool` 自 `pool_name`  
   - `missions`: 经 relations 反查，元素含 `canonical_name`、可选 zh 标题（最长/localization alias）、`contractor`、`system`（自 mission facts）  
   - `missing`: 若问句含材料/配方/制造且无 recipe 边 → `["crafting_recipe"]`  
3. Draft：蓝图/奖励 + 池；有 mission 则写「任务：{zh或名}（{contractor}，{system}）」类短句；有 `crafting_recipe` missing 则追加「库未收录制造配方/材料来源」。  
4. `confidence=high`；`browser.needed=false`；不追加软核实。

**验收**：

- CQ7 原句：draft 含蓝图/池；含 Battaglia 或「黑匣子」或任务 canonical；含材料缺失说明；`acquisition.shop_buyable=="unknown"`。  
- `JS-400 哪里买` 既有用例仍过。  
- `黄金在哪卖` 仍过。

---

## 5. Slice C — AGENTS + 端到端

- `AGENTS.md`：短码先 resolve；acquisition/missions 成文规则；材料 missing 不编造。  
- 手工/测试：原句端到端。

---

## 6. 协作流程

每 Slice：Cursor 改代码与测例 → 委托 Antigravity `gemini-3.8-flash`（role=test）跑 `uv run pytest` 相关用例并 **git commit** → Cursor 根据失败继续改。

---

## 7. 与既有 acquisition P0 关系

买/价路径保留；本期把触发面扩到获取/制造，并补 mission 编译。`shop_buyable` 语义不变。
