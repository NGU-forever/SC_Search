# Resolve 缺口修复：无数字英文名、中文俗称、基型蓝图优先

日期：2026-11-22  
状态：执行中（grill-me 默认拍板）  
前置：扩展复测报告 `docs/superpowers/reports/2026-11-22-cq7-obtain-retest.md` §5–6  
相关代码：`plan.py` token / `_prefer_blueprint_primary`；`data/seed_aliases.json`

---

## 1. 默认决策

| 缺口 | 默认方案 |
| :--- | :--- |
| 无数字英文名（Parallax / Coda / Demeco） | 在蓝图语境下额外抽取 **ASCII 词 token**（长度≥4），对每个 token 调 `resolve_name`；**仅保留有命中的 token**（停用词自然被过滤） |
| 中文俗称「视差」 | seed：`视差` → `Parallax Energy Assault Rifle`（slang）；可选 `视差步枪` |
| FS-9 皮肤抢 primary | blueprint 集合内再排序：**无引号皮肤名优先**、名称更短优先、与问句 token 更贴合优先 |
| FR-76 池无 mission | **本期不做**（数据无 `mission_reward_blueprint_pool`） |
| Live「卡拉克」 | seed：`卡拉克` → `Carrack`（与已有「克拉克」并列） |

协作：Cursor 开发；Antigravity `gemini-3.8-flash` 测与 commit。

---

## 2. 目标与非目标

### 2.1 目标

1. `Parallax 蓝图哪里弄，材料怎么得` → primary 为 `Parallax Energy Assault Rifle`（或同基型 blueprint），有材料。  
2. `视差步枪蓝图怎么获取…` → 命中同一基型 blueprint。  
3. `Coda 手枪蓝图…` / `Coda Pistol…` → `Coda Pistol` blueprint。  
4. `FS-9 蓝图…` / `FS-9 LMG…` → **优先** `FS-9 LMG`，不是 `FS-9 "Blacklist" LMG`。  
5. `卡拉克货舱多大` → 能解析到 Carrack（若库有船实体）。  
6. 既有 CQ7 / JS-400 / 黄金用例不回归。

### 2.2 非目标

- 全量中文枪名 slang 表。  
- 补全所有奖励池的 mission 边。  
- 改 MCP 签名。

---

## 3. 实现要点

### 3.1 Token（`plan.py`）

- 保留现有 digit shortcode。  
- 新增：蓝图语境下 `_NAME_TOKEN_RE = (?<![A-Za-z0-9])([A-Za-z]{4,16})(?![A-Za-z0-9])`。  
- 硬过滤小写停用词集合（`blueprint`, `craft`, `materials`, `where`, `what`, `rifle`, `pistol`, … 可选：若 resolve 无命中则丢弃，步枪词可保留给 resolve）。  
  **更稳默认**：抽取后一律 `resolve_name`；**零命中则丢弃**，不做大停用词表。  
- 「手枪」等中文不作为 ASCII token。

### 3.2 基型蓝图排序

在 `_prefer_blueprint_primary` 内对 `bps` 排序 key：

1. 名称含 `"` → 惩罚（皮肤）  
2. `len(canonical_name)` 升序  
3. 若问句 shortcodes/name tokens 中有 `t`，名称以 `t` 开头且更短者优先  
4. `canonical_name` 字典序

### 3.3 Seed

```json
{"alias": "视差", "target": "Parallax Energy Assault Rifle", "lang": "slang", "kind": "blueprint"},
{"alias": "视差步枪", "target": "Parallax Energy Assault Rifle", "lang": "slang", "kind": "blueprint"},
{"alias": "卡拉克", "target": "Carrack", "lang": "zh", "kind": "ship"}
```

同步 `data/seed_aliases.json` 与 `src/sc_search/data/seed_aliases.json`（若双份）。`sync aliases` 后 Live 生效；单测 fixture 直接 upsert。

---

## 4. 验收测试

| 测例 | 期望 |
| :--- | :--- |
| plan/answer：`Parallax 蓝图…材料` | primary blueprint 基型；有 materials |
| plan/answer：`视差步枪蓝图…`（seed 后） | 同上 |
| plan：`FS-9 LMG 蓝图…` / `FS-9 蓝图…` | primary `FS-9 LMG` |
| plan：`Coda 蓝图…材料` | primary `Coda Pistol` |
| answer：`卡拉克货舱`（有 Carrack+货舱 fact） | high / 货舱 |
| 回归：CQ7 / JS-400 / 黄金 | 通过 |

---

## 5. 文件

- `docs/superpowers/specs/2026-11-22-resolve-slang-base-blueprint-design.md`  
- `src/sc_search/query/plan.py`  
- `data/seed_aliases.json` (+ src 副本)  
- `tests/test_query_plan.py` / `tests/test_answer.py` / aliases seed 测若有  
