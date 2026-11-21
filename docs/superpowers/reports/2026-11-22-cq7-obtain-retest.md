# 复测记录：CQ7 蓝图获取 + 制造材料

日期：2026-11-22  
问句：`cq7的蓝图怎么获取以及我要制造的话，材料怎么得。`  
Live：`4.10.3`  
相关提交：`cb74619` → `dca4fac` → `5c4ad40`

---

## 1. 自动化测试

命令：`uv run pytest tests/test_answer.py tests/test_scw_blueprints.py tests/test_query_plan.py -v`

结果：**24 passed**（约 0.32s）

关键用例：

- `test_shortcode_cq7_blueprint_context_prefers_blueprint`
- `test_cq7_obtain_materials_compiles_mission_and_recipe`
- `test_apply_scw_blueprints_links_existing_scmdb_blueprint`
- `test_blueprint_buy_acquisition_unknown`（JS-400 不回归）

---

## 2. 分析过程（逐步）

### ① 理解

| 信号 | 值 |
| :--- | :--- |
| 主意图 | `blueprint` / 获取 + 制造材料 |
| shortcodes | `["cq7"]` |
| `_wants_obtain_or_price` | `true`（命中「蓝图/获取/材料」） |
| `_CRAFT_MATERIALS_RE` | `true` |

### ② 对照：修复前会失败的路径

| 路径 | 结果 |
| :--- | :--- |
| `resolve_name(整句)` | 0 命中 |
| embedded alias ⊆ 问句 | 0 命中（无短别名 `CQ7`） |
| 旧结论 | 「本地知识库未覆盖该问题对应实体」 |

### ③ 检索（现行）

1. Token `cq7` → `resolve_name("CQ7")`（多 item 皮肤 + blueprint）  
2. 蓝图语境 → **primary 提升为** `kind=blueprint` `CQ7 Rifle`（id 1479）  
3. `entity_facts` → `pool_name=BP_MISSIONREWARD_Battaglia_behr_rifle_ballistic_03`  
4. `mission_reward_blueprint_pool` → `Battaglia_RecoverItem_VeryHard` / 「非常危险的黑匣子回收」/ Recco Battaglia / Nyx  
5. `blueprint_requires`（scw_blueprint）→ Aluminum 0.06、Hephaestanite 0.02、Iron 0.01 SCU  

`query_plan`：`intent=facts`，`path=mcp_only`，`confidence=high`，entities≈12（blueprint 排第一）。

### ④ 成文

- `confidence=high`，`browser.needed=false`，`missing=[]`  
- `shop_buyable=unknown`（不断言绝对买不到）  
- 草稿含：池、任务线索、三料用量、资源采矿/精炼说明（矿点未收录）

**answer_draft：**

> CQ7 Rifle 本地仅见蓝图/任务奖励记录（池：BP_MISSIONREWARD_Battaglia_behr_rifle_ballistic_03），未找到商店报价；不能据此断言绝对买不到；任务线索：非常危险的黑匣子回收（Recco Battaglia，Nyx）；制造材料：Aluminum 0.06 SCU、Hephaestanite 0.02 SCU、Iron 0.01 SCU；资源类需采矿/精炼获取（具体矿点库未收录）（Live 4.10.3）。

---

## 3. 与首轮失败对比

| 项 | 首轮 | 本轮复测 |
| :--- | :--- | :--- |
| 实体 | 未命中 | `CQ7 Rifle` blueprint |
| 蓝图获取 | 无 | Battaglia 黑匣子任务 + 池名 |
| 材料 | 无 / 仅 missing | 三料 + SCU 用量 |
| confidence | low | high |
| 草稿病句 | 「未覆盖实体」 | 可执行短答 |

---

## 4. 仍未覆盖（诚实缺口）

- 各材料**具体矿点 / 精炼站**（草稿已声明未收录）  
- 材料品质对成品属性的数值表（未进 obtain 草稿）  
- MCP 进程若未重载，对话侧仍可能看到旧结果（需重启 sc-search）

---

## 5. 扩展复测矩阵（同类话术 · Live 4.10.3）

复测时间：同日后续轮次。问法均为「蓝图怎么获取 / 材料怎么得」类。

### 5.1 配件（组件）

| 问句 | primary | 池 / 任务 | 材料 | 结果 |
| :--- | :--- | :--- | :--- | :--- |
| JS-400 蓝图怎么获取，能造吗材料怎么得 | `JS-400` blueprint | `BP_REWARDS_FullStrikeOnStationB` · 急需战术打击编队（InterSec） | Beryl 0.5 / Savrilium 0.85 / Stileron 1.22 SCU | **通过** |
| JS-400 哪里买 | 同上 | 同上 | 同上（买点问法也走出 obtain） | **通过**（不断言可买） |
| FR-76 蓝图怎么获取，材料怎么得 | `FR-76` | `BP_REWARDS_Xenothreat2_85_02` · 任务未反查到 | Feynmaline ×58 / Stileron 1.1 SCU | **部分通过**（有料无任务名） |
| XL-1 量子引擎蓝图怎么弄，制造要啥材料 | `XL-1` | `BP_REWARDS_CovalexSuper` · Covalex Master 货运 | Borase 1.24 / Stileron 0.5 / Tungsten 0.5 SCU | **通过** |

### 5.2 武器

| 问句 | primary | 池 / 任务 | 材料 | 结果 |
| :--- | :--- | :--- | :--- | :--- |
| cq7的蓝图…材料怎么得 | `CQ7 Rifle` | Battaglia 黑匣子 | Al/Heph/Iron | **通过** |
| Parallax Energy Assault Rifle 蓝图…材料 | `Parallax Energy Assault Rifle` | Foxwell 橙色摧毁威胁 | Carinite×1 / Gold 0.02 / Tungsten 0.04 | **通过**（需完整英文名） |
| Coda Pistol 蓝图…材料 | `Coda Pistol` | Headhunters Deep space hit（Pyro） | Al 0.02 / Heph 0.01 / Iron 0.01 | **通过**（需完整英文名） |
| FS-9 蓝图怎么获取，制造材料呢 | `FS-9 "Blacklist" LMG`（误） | 无池 | Heph/Iron/Lindinium | **错实体**（短码命中皮肤蓝图） |
| FS-9 LMG 蓝图…材料 | 仍常落到 Blacklist | — | 同上 | **错实体**（blueprint 排序偏皮肤名） |
| 视差步枪蓝图怎么获取… | 无实体 | — | — | **失败**（中文俗称未进短码；无「视差→Parallax」口语映射） |
| Parallax 蓝图哪里弄… | 无实体 | — | — | **失败**（无数字，短码不抽词；整句/embedded 不命中） |
| Coda 手枪蓝图…（无 Pistol） | 无/错 | — | — | **失败**（同上） |
| Demeco / Karna 蓝图… | 无实体 | — | — | **失败**（无数字短码） |

### 5.3 对照（不应走 acquisition）

| 问句 | 结果 | 备注 |
| :--- | :--- | :--- |
| 黄金在哪卖 | prices · Gold · high | **正常** |
| 卡拉克货舱多大 | 未覆盖实体 · low | **异常**：Live 库缺「卡拉克」别名（与 fixture 测例无关） |

---

## 6. 扩展复测结论与新缺口

**已稳：** 带数字短码的配件（JS-400 / FR-76 / XL-1）及 CQ7；完整英文蓝图名的武器（Parallax Energy Assault Rifle / Coda Pistol）。

**新缺口（按优先级）→ 见 spec `2026-11-22-resolve-slang-base-blueprint-design.md`：**

1. **无数字的英文枪名短码**（Parallax / Coda）→ 蓝图语境字母 token + resolve  
2. **中文俗称**（视差）→ seed `视差` / `视差步枪`  
3. **皮肤蓝图抢 primary**（FS-9）→ 无引号/更短基型优先；蓝图语境强制合并 blueprint LIKE 命中  
4. **FR-76** 池无 mission → 本期不做（数据缺口）  
5. **卡拉克** → seed 补别名  

### 6.1 缺口修复后冒烟（aliases sync 后）

| 问句 | primary | 材料 | 判定 |
| :--- | :--- | :--- | :--- |
| Parallax 蓝图…材料 | Parallax Energy Assault Rifle | Carinite/Gold/Tungsten | 通过 |
| 视差步枪蓝图… | 同上 | 同上 | 通过 |
| Coda 蓝图…材料 | Coda Pistol | Al/Heph/Iron | 通过 |
| FS-9 / FS-9 LMG 蓝图… | FS-9 LMG | Heph/Iron/Lindinium | 通过 |
| 卡拉克货舱多大 | Anvil Carrack | 456 SCU | 通过 |
