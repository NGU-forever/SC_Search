# 知识库 P0：索引、同步事务与 UEX 快照门禁

状态：已实施（索引 / 事务 / UEX 快照替换 / 价格 stale 措辞门禁已落地；待 Claude 复测与提交）  
日期：2026-11-26  
优先级：P0（知识库完整性第一阶段）  
范围：本地 SQLite 热路径索引、按来源事务化同步、UEX 价格快照替换与 stale 回答门禁  
依据：GPT-5.6 对 `data/sc_knowledge.sqlite` 的只读分析（约 16MB；facts/relations 无索引；UEX 报价仍为 3.24.2）  
关联阶段：[answer_quality_optimization.md](answer_quality_optimization.md)（成文）、[classification_maintainability_optimization.md](classification_maintainability_optimization.md)（分类）、[knowledge_coverage_optimization.md](knowledge_coverage_optimization.md)（覆盖扩展）

## 1. 背景

本地库已经是问答唯一证据源：上游 SCMDB / SCW / UEX / Wiki 经 ingest 落入统一表后，`sc_ask` 只读 SQLite。当前瓶颈不是容量，而是：

1. 热路径查询在缺索引时全表扫描或依赖临时自动索引；
2. 同步在自动提交模式下 clear/upsert，失败会留下半套数据；
3. UEX 只做按键 upsert，上游已删除的报价会残留；且报价 `game_version` 落后 Live 时仍可能被表述为“当前最佳价”。

本阶段目标是把「外源收集 → 本地可替换快照」的更新语义做硬，使后续每次 `sync` 都更像刷新**自己的知识库版本**，而不是半合并的第三方镜像。

心智模型（本阶段只强化 L0→L1）：

```text
L0 上游原始证据     SCMDB / SCW / UEX / Wiki
L1 本地规范层       索引、事务、快照替换、stale 门禁   ← 本文
L2 策展层           seed_aliases / guides（后续）
```

## 2. 问题定义

### 2.1 热路径缺索引

现状（`go/internal/ingest/schema.sql`）：

| 表 | 已有索引 | 缺口 |
| --- | --- | --- |
| `entities` | `(kind,uuid,version)`、`(kind,canonical_name,version)` | `COLLATE NOCASE` 的 canonical 查找无法命中 |
| `aliases` | `alias`、唯一 `(entity,alias,lang,source)` | 查询使用 `alias = ? COLLATE NOCASE`，二进制索引不命中 |
| `facts` | 无 | `EntityFacts`、任务多 JOIN（`missions.go`）全表/自动索引 |
| `relations` | 无 | 双向关系扫描全表 |
| `prices` | 最新键、commodity/side、terminal | 最优价 `ORDER BY price` 缺复合索引 |

实库量级约：entities 14k、aliases 31k、facts 39k、relations 56k、prices 2.9k。库小，但任务查询对 `facts` 多次 JOIN，缺索引的代价已经可测。

### 2.2 同步非原子

典型路径（以 SCMDB / SCW / Wiki 为例）：

```text
StartRun → ClearSource* → 逐行 Upsert/Insert → FinishRun(ok|failed)
```

`ClearSourceFactsRelations` / `ClearSourceDocuments` 与后续写入均在 SQLite 自动提交模式。中途失败时：

- 旧数据可能已删；
- 新数据只写了一部分；
- `ingest_runs` 记 `failed`，但库内容已不可回滚。

UEX 路径甚至不做来源级 clear：只 `ReplacePrice` upsert，**上游下架的报价永久残留**（分析时已见旧 run 遗留的 `Waste` 报价）。

### 2.3 UEX 版本落后与回答语义

- Live（SCMDB）约 `4.10.3`，全部 UEX 报价仍为 `3.24.2`。
- `AttachFreshness` / Presenter 已能标 `stale`，但价格旁路与贸易成文仍可能把结果读成「当前最佳价」。
- 缺少「本源本版本快照替换」时，即使上游修好，本地脏行也会继续污染最优价。

## 3. 目标与非目标

### 3.1 目标

1. 为 facts / relations / aliases / entities / prices 热路径补齐索引；现有库与新库都能生效。
2. 每个 ingest 来源在「下载成功之后」的写库阶段进入**单事务**：clear → upsert → commit；失败 rollback，并单独留下 `failed` run 记录。
3. UEX 成功同步后，删除本来源、本 `game_version` 下本次未出现的 `prices` 行（快照替换）。
4. 价格类回答在 `meta.stale=true` 时必须显式降级措辞，禁止称为当前 Live 最佳价。
5. 保持 MCP 工具契约与现有表结构兼容；不引入向量库、分库或每次 `VACUUM`。

### 3.2 非目标

- 不做完整 `source_snapshots` / tombstone / 稳定跨版本实体身份（属 P1）。
- 不拆 `mission_profiles` / 通用 `offers` 读模型（属 P1）。
- 不在本阶段做 SCW 别名污染大清洗（可紧随其后的小 P0.5，不阻塞本文件三项）。
- 不扩展社区知识覆盖（见 knowledge_coverage P2）。
- 不改变 `sc_ask` 状态机骨架与 Presenter 投影器架构。

## 4. 方案设计

### 4.1 热路径索引

#### 4.1.1 DDL（写入 `schema.sql`，并由 Init/迁移应用）

```sql
CREATE INDEX IF NOT EXISTS idx_facts_entity_key
  ON facts(entity_id, key);

CREATE INDEX IF NOT EXISTS idx_relations_from_type_to
  ON relations(from_id, rel_type, to_id);

CREATE INDEX IF NOT EXISTS idx_relations_to_type_from
  ON relations(to_id, rel_type, from_id);

CREATE INDEX IF NOT EXISTS idx_aliases_alias_nocase
  ON aliases(alias COLLATE NOCASE, entity_id);

CREATE INDEX IF NOT EXISTS idx_entities_canonical_nocase
  ON entities(canonical_name COLLATE NOCASE, kind);

CREATE INDEX IF NOT EXISTS idx_prices_best
  ON prices(commodity_entity_id, side, price);
```

说明：

- 全部使用 `IF NOT EXISTS`，幂等。
- **暂不加** `facts(entity_id,key,source,game_version)` 唯一约束：需先盘点多值 key；本阶段只优化读。
- 预计索引体积数 MB 级，写入略增；对 16MB 库可接受。

#### 4.1.2 现有库迁移

`InitDB` 只对空库可靠。已有 `sc_knowledge.sqlite` 需要一条安全迁移路径：

1. 在 `ingest init` / 打开库后调用 `EnsureIndexes(conn)`，执行上述 DDL；
2. 或 `sc-search ingest migrate` 子命令（可选，若不想塞进 init）；
3. 迁移后对开发库跑一次 `ANALYZE`（不跑 `VACUUM`）。

验收：对 `EntityFacts`、任务列表、双向 relations、`ResolveName` exact、`BestPrices` 用 `EXPLAIN QUERY PLAN` 确认走新索引（或至少不再全表扫 facts/relations）。

### 4.2 同步事务化

#### 4.2.1 统一写库协议

对每个来源 `Sync*Live` / `Sync*Payload`：

```text
1. 网络/文件读取（在事务外）
2. StartRun(status=running)          -- 可在事务外，便于失败审计
3. BEGIN
4.   clear（按源约定）
5.   apply payload
6. COMMIT 或 ROLLBACK
7. FinishRun(ok|failed, row_count, notes, game_version)
```

约束：

- **下载失败**：不 clear、不开写事务；只把 run 标 failed（若已 StartRun）。
- **写库失败**：rollback 后 FinishRun failed；库内容保持上一成功快照。
- **成功**：commit 后 FinishRun ok；读者只看见完整快照。
- `StartRun`/`FinishRun` 与业务数据的事务边界二选一（推荐）：
  - **A（推荐）**：run 元数据在事务外更新；业务 clear/apply 在独立事务内。失败时上一版业务数据仍在，run 仍记录 failed。
  - **B**：整包含 FinishRun 同事务。实现更简单，但 rollback 时可能丢 failed 审计行——若选 B，失败后必须再插入一条 failed run。

本阶段采用 **A**。

#### 4.2.2 各源 clear 策略（本阶段）

| 来源 | 现有 clear | 本阶段要求 |
| --- | --- | --- |
| `scmdb` | `ClearSourceFactsRelations(source, game_version)` | 包进事务 |
| `scw_api` / `scw_blueprint` | 已有 clear / delete by source | 包进事务 |
| `wiki` | `ClearSourceDocuments` | 包进事务 |
| `localization` | 按现有逻辑 | 包进事务 |
| `aliases` | seed upsert | 包进事务（无破坏性 clear 则仅包写入） |
| `uex` | **无来源级 clear** | 见 4.3 快照替换，整体仍在单事务 |

辅助形态建议：

```go
func WithTx(conn *sql.DB, fn func(tx *sql.Tx) error) error
```

并把 `UpsertEntity` / `UpsertFact` / `ReplacePrice` / `Clear*` 逐步改为接受 `DBTX`（`*sql.DB | *sql.Tx`）接口，避免复制两套 SQL。

#### 4.2.3 并发与 PRAGMA

- 同步期间可设 `PRAGMA busy_timeout`；不在本阶段改 WAL 策略，除非实测锁等待。
- MCP 只读查询与同步并行时，事务提交应瞬时可见完整快照；不引入连接池多写。

### 4.3 UEX 快照替换 + stale 门禁

#### 4.3.1 快照替换语义

成功拉取 payload 后，在同一写事务内：

1. Upsert terminals / commodities / vehicles（实体与别名，保持现有逻辑）；
2. Upsert 本次出现的全部 `prices`（含商品与飞船购点）；
3. **删除残留**：

```sql
DELETE FROM prices
WHERE source = 'uex'
  AND game_version = ?
  AND ingest_run_id != ?;
```

或等价：先记下本次写入的 `(commodity,terminal,side)` 集合，删除同 source+version 且不在集合中的行。

推荐用 `ingest_run_id`：实现简单，且与 `ReplacePrice` 已写入的 run 绑定一致。

注意：

- 仅删 `prices`，不在本阶段大规模删 UEX 产生的 terminal/commodity 壳实体（壳实体治理属后续）。
- `game_version` 变化时：旧版本价格行是否保留？
  - **本阶段默认**：只替换「本次写入的那个 `game_version`」；其它历史 version 行暂留。
  - 若产品上只要「现行 UEX 一份价」，可在成功后额外 `DELETE FROM prices WHERE source='uex' AND game_version != ?`——作为可配置选项，默认关闭，避免误伤对照数据。

#### 4.3.2 stale 门禁（读路径）

已有能力：

- `query.AttachFreshness`：可比版本 ≠ Live ⇒ `stale=true`
- Presenter：`meta.stale` → Warnings + draft 提示

本阶段补强：

| 路径 | 要求 |
| --- | --- |
| `sc_best_prices` / `sc_terminal_prices` meta | 必须 `AttachFreshness`；`stale` 与 `as_of` 稳定存在 |
| `sc_answer` / `sc_ask` 贸易、单货物价格旁路 | draft **禁止**在 stale 时使用「当前最佳价 / 现行市价」等无条件措辞 |
| Presenter trade / 价格块 | 标题或首句标明 `数据版本 {gv}（落后 Live {as_of}）` |
| 黄金评测 | 增加至少 1 条：UEX stale 时 draft 含陈旧提示且不含「当前最佳」 |

可选策略（默认采用 **标注保留结果**，不做硬拒绝）：

- **标注保留（默认）**：仍返回价格数字 + 强制 stale 文案；
- **硬拒绝（可选 flag）**：`SC_SEARCH_STALE_PRICES=reject` 时价格旁路返回 missing/警告、不给数字——仅调试用，正式默认不开启。

#### 4.3.3 与「自己的知识库」的关系

快照替换 + stale 门禁的产品语义是：

- 每次成功的 UEX sync = 你库里的一版**完整价格快照**；
- 过期快照可以留下数字，但**不能假装是现行 Live**；
- 读者信任的是本地策略，而不是上游页面的即时状态。

## 5. 文件与实现边界

```text
go/internal/ingest/
├── schema.sql              # 追加索引 DDL
├── db.go                   # EnsureIndexes / Open 后迁移钩子
├── normalize.go            # DBTX、WithTx、Clear* 签名
├── uex.go                  # 事务 + 快照替换删除
├── scmdb.go / scw.go / …   # Sync* 写库段包事务
└── *_test.go               # 事务回滚、残留价删除、索引存在

go/internal/query/
├── freshness.go            # 已有；必要时补价格专用 helper
├── prices.go               # meta 保证 AttachFreshness
└── answer*.go              # stale 措辞门禁

go/internal/orchestrate/
└── present*.go             # 价格/贸易 stale 标题（若旁路未覆盖）

go/internal/eval/questions.jsonl
└── 至少 1 条 stale 价格严格题
```

CLI：`ingest init` 调用 `EnsureIndexes`；`scripts/sync_all.sh` 无需改流程，行为随二进制升级。

## 6. 实施顺序

1. **索引**：改 schema + `EnsureIndexes` + 计划检查测试。
2. **DBTX + WithTx**：先改 normalize 与一两个源，再铺开。
3. **UEX 快照替换**：事务内 upsert + 按 `ingest_run_id` 删残留；单测用两版 payload。
4. **其它源事务化**：scmdb / scw / wiki / localization / aliases。
5. **stale 门禁**：价格旁路 + Presenter + 黄金题。
6. **回归**：`go test ./...`；可选对实库 `EXPLAIN QUERY PLAN` 抽样。

## 7. 测试与验收

### 7.1 单测

| 用例 | 期望 |
| --- | --- |
| `EnsureIndexes` 幂等 | 二次调用不报错 |
| 写库中途错误 | rollback 后业务表与同步前一致；存在 failed run |
| UEX payload A → B（B 少一条报价） | A 中消失的那条价在成功后不存在 |
| UEX 写到一半失败 | 旧快照完整保留 |
| stale 价格旁路 | `meta.stale=true`；draft 含陈旧说明；不含「当前最佳价」 |
| 任务 / EntityFacts 计划（可选） | EXPLAIN 显示使用 `idx_facts_entity_key` 等 |

### 7.2 手工 / MCP 冒烟

```text
1. sc-search ingest init          # 或打开库触发 EnsureIndexes
2. ingest sync uex（有 Token）或 SyncUexPayload 夹具
3. sc_best_prices 任意货物
4. sc_ask「XX 在哪卖」——若 UEX 版本 ≠ Live，必须出现 stale 提示
5. sc_ask 任务手续费题 —— 确认索引变更未破坏成文
```

### 7.3 完成定义

- [x] 六条索引存在于 schema 与实库（`ingest init` / 每次 `ingest sync` 都会幂等应用；实库 16MB→约 22MB）
- [x] 主要 Sync* 写库路径事务化，失败可回滚（scmdb / scw / scw-blueprints / wiki / localization / aliases / uex）
- [x] UEX 成功同步无「幽灵报价」（按 `ingest_run_id` 删残留；空报价回包不清库；`SC_SEARCH_UEX_PRUNE_OTHER_VERSIONS=1` 可选清旧版本）
- [x] stale 价格回答措辞门禁有单测 + 黄金题防护（`must_not_contain: 当前最佳/现行市价`）
- [x] `go test ./...` 全绿
- [x] 不引入 VACUUM / 向量 / 表大重构

### 7.4 实现备注（与设计的差异）

- 事务边界采用方案 A：`StartRun`/`FinishRun` 在事务外，业务 clear+apply 在 `applyInTx` 内；失败后上一版快照保持完整。
- 写 helper 统一接受 `DBTX`（`*sql.DB` / `*sql.Tx`）；`query.ResolveName` 改接 `Queryer` 以便 aliases 同步在事务内解析。
- stale 门禁：`sc_ask` 价格旁路在快照版本落后 Live 时，措辞改为「快照中较优」并强制附带「非当前 Live 报价」，同时写入 `evidence.meta.stale`；未实现硬拒绝 flag（保持默认标注保留数字）。
- 现有实库的 UEX 仍是 3.24.2，所以价格类回答现在会明确标注陈旧；配置 `UEX_API_TOKEN` 后重新 `sync uex` 才会刷新。

## 8. 风险与回退

| 风险 | 缓解 |
| --- | --- |
| NOCASE 索引与现有唯一索引并存导致写入变慢 | 只加读索引；观察 sync 耗时 |
| 长事务锁住 MCP 只读 | 下载在事务外；写事务尽量短；必要时 busy_timeout |
| 按 run_id 删除误伤并行 UEX sync | CLI 同步单进程；禁止并行 `ingest sync uex` |
| stale 文案过严影响体验 | 默认标注保留数字，不硬拒绝 |
| 旧库未跑 EnsureIndexes | init/open 强制执行；文档写明 Release 用户升级后重载 MCP |

回退：索引可 `DROP INDEX`；事务与快照替换为行为修复，回退即回到半更新风险，不建议长期回退。

## 9. 明确延后（不在本文实施）

| 项 | 去向 |
| --- | --- |
| SCW 分类词污染别名（「可飞行」等） | P0.5 / 紧随的别名清洗小任务 |
| `mission_profiles`、`offers`、终端归属关系 | 知识库 P1 |
| `source_snapshots` + tombstone + 稳定实体 ID | 知识库 P1 |
| 社区 howto / 客户端文档覆盖 | [knowledge_coverage_optimization.md](knowledge_coverage_optimization.md) |
| 每次同步 `VACUUM`、向量检索、分片 | 不做 |

## 10. 建议开发流程

与回答质量阶段相同：

1. Cursor 按本文实现；
2. Cursor Claude 跑 `go test ./...` + UEX/stale/任务冒烟；
3. 通过后提交（不主动 push，除非明确要求）。

开工入口：先落地 `EnsureIndexes` + UEX 事务快照替换，再铺其它源事务与 stale 措辞门禁。
