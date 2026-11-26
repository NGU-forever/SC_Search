# 工程质量与解析准确性优化（CI / 测试夹具 / 评测集 / 别名清洗）

状态：待实施  
日期：2026-11-26  
优先级：P0（工程保障，紧随库 P0）  
范围：持续集成门禁、脱离本机数据的测试夹具、评测集扩容、SCW 分类词别名污染清洗  
前置：[knowledge_db_p0_optimization.md](knowledge_db_p0_optimization.md)（索引 / 事务 / UEX 快照，已实施）  
关联：[answer_quality_optimization.md](answer_quality_optimization.md)、[classification_maintainability_optimization.md](classification_maintainability_optimization.md)、[knowledge_coverage_optimization.md](knowledge_coverage_optimization.md)

## 1. 背景

数据库与状态机的结构问题已处理或有专门文档。剩下的主要风险不在功能，而在**改动是否被可靠验证**，以及**名字解析是否被脏别名污染**：

1. 仓库没有 push / PR 门禁；`.github/workflows/` 只有 `release.yml`，仅在 `v*` tag 或手动触发时运行，且不含 `go test` / `go vet`。
2. 有 14 处 `t.Skip` 分布在 7 个测试文件里，核心问答与评测依赖本机的 `data/sc_knowledge.sqlite`。没有库的环境（干净克隆、CI）里这些测试被跳过，结果是“全绿但没测”。
3. 黄金评测只有 20 题，偏路由断言；任务手续费、报酬区间、stale 措辞这类问题，是靠人工追问才发现的。
4. SCW 同步把 `size / type / production_status / career / foci` 的中文标签写成了飞船别名，造成大范围解析冲突。

本阶段目标：让每次改动自动被验证，让关键行为有不依赖实库的回归保护，并修掉当前最伤解析准确率的脏数据来源。

## 2. 问题定义

### 2.1 无 CI 门禁

现状：

| 触发 | 工作流 | 是否跑测试 |
| --- | --- | --- |
| push / PR | 无 | 否 |
| `v*` tag / 手动 | `release.yml` | 否（只构建、同步库、打包） |

后果：编译错误、`go vet` 问题、测试回归只能靠本地发现；Release 前也没有最后一道检查。

### 2.2 测试依赖本机实库

现状：

- `orchestrate.testEngine`、`query` 下多个测试用 `db.Open(root)`，打不开就 `t.Skip`。
- 黄金评测 `TestAskGoldQuestionsStrict` 经由 `testEngine`，无库时整条被跳过。
- 同一份测试在有库的开发机和无库的 CI 上覆盖面完全不同，且断言绑定实库当天的数据（例如 UEX 版本、某实体是否存在），数据一同步就可能变化。

已有的好范式：库 P0 新增的 `ingest/p0_test.go` 和 `query/stale_price_test.go` 都用临时库，不依赖实库。

### 2.3 评测集过薄

- 20 题；多数只断言 `expected_primary` 与少量 `must_contain`。
- 缺少：数值断言、反例（不该出现的词）、多槽与无匹配、stale、浏览器门禁的系统覆盖。
- 社区高频问法已整理在 [community_question_catalog_zh.md](community_question_catalog_zh.md)，没有转成可执行用例。

### 2.4 SCW 分类词别名污染

根因（`go/internal/ingest/scw.go`，`ApplyScwVehicles`）：

```go
for _, pair := range [][2]string{{"size","size"},{"type","type"},{"production_status","production_status"},{"career","career"}} {
    ...
    if zh != "" && zh != en {
        _ = UpsertAlias(conn, entityID, zh, "zh", scwAPISource)   // 分类标签被当成飞船别名
    }
}
// foci 的 zh 标签同样被写成别名
```

实库现状（SCW 来源的中文别名共 941 条，被多实体共享的归一化别名共 2552 个）：

| 别名 | 命中实体数 |
| --- | --- |
| 可飞行 | 232 |
| 小 | 116 |
| 打击 | 107 |
| 中等 | 55 |
| 大 | 39 |
| 运输船 | 35 |
| 勘探 | 31 |
| 轻型战斗机 | 26 |

后果：用户说“勘探”“运输船”“可飞行”时，`ResolveName` 会命中几十艘船，排序只能按种类和名称，容易挤掉真正想问的实体，也让下游槽位填充变噪。

这些词是**属性取值**，应存为 facts / facet，而不是实体别名。

## 3. 目标与非目标

### 3.1 目标

1. push 与 PR 自动运行 `go build`、`go vet`、`go test ./...`。
2. 核心问答、评测和解析测试不再依赖本机实库；无实库环境测试不被静默跳过。
3. 评测集从 20 扩到 60–100 题，增加数值、反例、stale、多槽和门禁覆盖。
4. SCW 分类标签不再进入 `aliases`；已写入的污染别名被清理；解析冲突数量显著下降。
5. 保持 MCP 契约、表结构和现有回答行为兼容。

### 3.2 非目标

- 不引入外部测试框架或服务。
- 不在 CI 里联网同步真实数据。
- 不做 LLM 评测；仍然是确定性字符串 / 字段断言。
- 不在本阶段重做 `ResolveName` 排序算法（别名清洗后再按数据评估）。
- 不改动 Release 打包的数据来源（见 8 延后项）。

## 4. 方案设计

### 4.1 CI 门禁

新增 `.github/workflows/ci.yml`：

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go/go.mod
          cache-dependency-path: go/go.sum
      - name: Build
        run: cd go && CGO_ENABLED=0 go build ./...
      - name: Vet
        run: cd go && go vet ./...
      - name: Test
        run: cd go && go test ./... -count=1
```

要点：

- 使用 `go/go.mod` 指定版本，与 `release.yml` 一致。
- 不联网、不需要 `UEX_API_TOKEN`。
- `gofmt` 检查先**不**加：仓库里已有多处历史未格式化文件（例如 `ingest/scw.go`、`orchestrate/present.go`），先单独做一次格式化提交，再把 `gofmt -l` 加入门禁，避免首次 CI 即红。
- 后续可在 `release.yml` 的 `prepare-db` 之前加同样的测试步骤，作为发布前最后检查。

### 4.2 测试夹具：脱离实库

#### 4.2.1 原则

- **单元和行为测试** 使用临时库 + 固定夹具；断言绑定夹具内容，不绑定实库当天数据。
- **实库冒烟测试** 仍保留，但必须显式标记，不得替代夹具测试。
- 无实库时：夹具测试照常运行；实库冒烟测试 `Skip` 并带明确原因（仅作为补充）。

#### 4.2.2 夹具构造

新增内部测试辅助包（仅测试使用），提供：

```go
// go/internal/testfixture
func New(t *testing.T) *sql.DB      // 建临时库、InitDB、写入固定夹具
func Root(t *testing.T) string      // 夹具所需的 guides / seed 路径
```

夹具内容（保持小，几十行级别，覆盖断言所需）：

| 域 | 内容 |
| --- | --- |
| 实体与别名 | 2–3 艘船（含中文俗称）、2 个商品、2 个终端、2 个势力 |
| 价格 | 带版本的 UEX 价（同时提供与 Live 一致 / 落后两种变体） |
| 任务 | 若干含 `cost_uec / reward_uec / cooldown / contractor / systems` 的任务，覆盖手续费、报酬区间、声望门槛 |
| 蓝图 | 1–2 个蓝图及 `blueprint_requires` 关系 |
| 新鲜度 | `scmdb` 的 ok run，决定 Live 版本 |
| 文档 | 少量 `documents`（供 docs 路径） |

实现方式：优先复用 `ingest.SyncScmdbPayload / SyncUexPayload / ApplyScw*` 写入夹具，这样夹具本身也验证写库路径；个别难以构造的行用直接 SQL。

注意依赖方向：`ingest` 依赖 `query`，因此 `testfixture` 可以依赖 `ingest`，但 `query` 内部（`package query`）测试不能 import 它。需要夹具的 `query` 测试改为外部测试包（`package query_test`），与 `stale_price_test.go` 相同。

#### 4.2.3 迁移顺序

1. 先做 `orchestrate.testEngine` 与黄金评测：这是 `t.Skip` 影响最大的一组。
2. 再迁移 `query` 下 `answer_test / prices_test / resolve_test / plan_test` 中依赖具体实库数据的断言。
3. 保留一个 `TestLiveDBSmoke`：有实库就跑核心几问，无则 `Skip("live DB not present")`；CI 不依赖它。

#### 4.2.4 评测数据双轨

| 轨道 | 数据 | 用途 | 位置 |
| --- | --- | --- | --- |
| 夹具评测（CI） | 夹具库 | 回归保护；断言数值与措辞 | `eval/fixture_questions.jsonl` |
| 实库评测（本地） | 实库 | 发现数据与路由的真实问题 | `eval/questions.jsonl`（现有） |

夹具评测的断言可以写死具体数值，因为夹具固定；实库评测只写与数据版本无关的断言（延续这次给价格题加 `must_not_contain` 的做法）。

### 4.3 评测集扩容

#### 4.3.1 用例字段

沿用现有 `goldCase`：`question / expected_primary / must_contain / must_not_contain / must_have_fact_labels / must_not_browse / must_wait_confirm`。

按需追加（向后兼容，缺省忽略）：

| 字段 | 含义 |
| --- | --- |
| `expected_status` | `completed / partial / awaiting_user` 等 |
| `must_contain_any` | 同义写法任一命中即可 |
| `tags` | 分组，如 `price` `mission` `stale` `no_match` `gate` |

#### 4.3.2 覆盖矩阵（目标 60–100 题）

| 类别 | 数量 | 重点断言 |
| --- | --- | --- |
| 船参 / 实体 | 8–10 | 货舱等数值；中英俗称 |
| 价格 / 贸易 | 10–12 | 买卖方向；stale 措辞；禁用“当前最佳 / 现行市价” |
| 任务手续费 / 冷却 | 10–12 | 数值与单位；无势力不倾倒全表 |
| 任务报酬 / 声望 | 8–10 | 区间；势力过滤；截断警告 |
| 蓝图 / 制作 | 6–8 | 材料与数量 |
| 无匹配 / 缺失 | 6–8 | `no_match`、不编造数字 |
| 浏览器门禁 | 6–8 | 改装 / 采矿需确认；事实题不开浏览器 |
| 分类冲突 | 6–8 | 动作词优先于实体词（延续 DetectAction 反例） |
| 数据新鲜度 | 3–4 | Live 对照、同步时间 |

来源：`docs/community_question_catalog_zh.md` 的高频问法 + 近期人工追问中暴露的失败案例（手续费无“劫案”关键词、fee 与 reward 混问、无势力任务问等）。

#### 4.3.3 新用例准入

- 每个已修复的缺陷，必须随修复补一条能复现的评测题。
- 每新增一条分类规则，附中文正例、英文正例、冲突反例（与 classification 文档一致）。

### 4.4 SCW 别名清洗

#### 4.4.1 写入侧修复

在 `ApplyScwVehicles` 中：

1. `size / type / production_status / career / foci` 的中文值**不再** `UpsertAlias`；
2. 改为写 facts：保留现有英文 fact，并新增对应 `*_zh` fact（例如 `size_zh`、`type_zh`、`career_zh`、`foci_zh`），保证中文信息不丢失；
3. 仍写入的别名只限**实体自身名称**：`name`、`game_name`、`slug`。

#### 4.4.2 存量清理

在 SCW 同步事务内（沿用库 P0 的 `applyInTx`），重建前先清掉本来源别名：

```sql
DELETE FROM aliases WHERE source = 'scw_api';
```

然后由本次 apply 重新写入合法别名。这样重跑 `sync scw` 即可清除存量污染，无需单独迁移脚本。

对没有 token 或无法联网的环境，另提供一次性清理（可选，作为 `ingest` 子命令或测试辅助）：只删除 `source='scw_api' AND lang='zh'` 且该别名指向 ≥ N 个不同实体的行，N 默认 5；并输出被删清单。

#### 4.4.3 同类别名守卫：已移除

原计划在 `UpsertAlias` 之外增加写入前检查（同一来源内别名将指向多个不同实体且不是任何实体的 `canonical_name / name / slug` 时拦截并计数，写入 `ingest_runs.notes`）。实现后确认该守卫是**死代码**：写入侧修复后 `ApplyScwVehicles` 只写实体自身的 `name / game_name / slug`，且每次同步按来源 `DELETE FROM aliases WHERE source = 'scw_api'` 全量重建，SCW 路径已不可能产生跨实体共享的通用别名。因此移除守卫、`ScwVehicleStats` 及 `ingest_runs.notes` 相关 plumbing，不保留仅供守卫使用的测试。

保留的防线：分类中文标签只写 `*_zh` facts；只写自身名别名；按来源重建。若未来其他来源写入通用别名，再针对该来源补守卫。

#### 4.4.4 验收指标

以实库同步前后对比：

| 指标 | 现状 | 目标 |
| --- | --- | --- |
| 多实体共享的归一化别名数 | 2552 | 显著下降（先看分布，不设死数字） |
| “可飞行 / 小 / 打击”命中船数 | 232 / 116 / 107 | 0（或仅限真实同名实体） |
| 船舶中文俗称解析（seed 与 localization 来源） | 正常 | 不退化 |
| 黄金评测与解析测试 | 通过 | 通过 |

## 5. 文件与实现边界

```text
.github/workflows/
└── ci.yml                         # 新增

go/internal/testfixture/           # 新增：夹具库构造（仅测试用）
├── fixture.go
└── fixture_test.go

go/internal/orchestrate/
├── ask_test.go                    # testEngine 改用夹具
└── eval_test.go                   # 夹具评测 + 实库评测两条

go/internal/query/
└── *_test.go                      # 依赖实库数据的断言迁移到 query_test + 夹具

go/internal/ingest/
├── scw.go                         # 分类标签改写 facts；清本源别名（无别名守卫）
└── scw_test.go                    # 新增：分类词不入别名、*_zh facts 存在

eval/
├── questions.jsonl                # 实库评测（保留，扩展）
└── fixture_questions.jsonl        # 夹具评测（新增，CI 使用）
```

## 6. 实施顺序

1. **CI**：加 `ci.yml`，先让现有测试在 CI 上跑通；记录当前哪些测试因缺库被跳过。
2. **别名清洗**（独立于夹具，价值直接）：改 `ApplyScwVehicles`，补 `scw_test.go`，对实库重跑 `sync scw`，记录前后指标。
3. **夹具包**：`testfixture` + 迁移 `testEngine` 与黄金评测。
4. **评测扩容**：先扩夹具评测到约 60 题，再补实库评测中与数据版本无关的断言。
5. **迁移其余 query 测试**，保留 `TestLiveDBSmoke`。
6. **收尾**：统一格式化提交后，把 `gofmt -l` 加入 CI；在 `release.yml` 的打包前加入测试步骤。

其中第 1、2 步互不依赖，可并行。

## 7. 测试与验收

### 7.1 单测与评测

| 用例 | 期望 |
| --- | --- |
| `ApplyScwVehicles` 含 size/type/career/foci 中文值 | 对应中文值不在 `aliases`；`*_zh` facts 存在 |
| SCW 只写自身名别名并按来源重建 | 不存在跨实体共享的通用别名；守卫已移除 |
| 重跑 `sync scw` | 本源旧别名被清除后重建；合法船名别名仍在 |
| 夹具库 `ResolveName("勘探")` | 不命中船舶集合 |
| 无实库环境运行 `go test ./...` | 夹具测试与夹具评测全部执行，不被跳过 |
| 夹具评测价格题（落后 Live） | 含“落后 Live / 非当前 Live 报价”，不含“当前最佳 / 现行市价” |
| 夹具评测任务题 | 手续费、冷却、报酬区间数值与夹具一致 |

### 7.2 手工验证

```text
1. 对实库副本：记录“多实体共享别名数”与“可飞行/小/打击”命中数
2. sc-search ingest sync scw
3. 重新统计，对比目标指标
4. MCP 抽问：勘探 / 运输船 / 卡拉克 / 舒宾，确认解析与回答未退化
5. 推一个故意失败的 PR，确认 CI 红；修复后转绿
```

### 7.3 完成定义

- [ ] `ci.yml` 在 push 与 PR 上运行 build / vet / test，并在干净克隆上通过
- [ ] 夹具评测在无实库环境中完整执行，核心测试不再因缺库被跳过
- [ ] 评测集不少于 60 题，覆盖矩阵各类至少 3 题，并含反例
- [ ] SCW 分类标签不再进入别名；存量污染清理；守卫已移除（只写自身名别名并按来源重建）；指标达标
- [ ] 解析与问答行为无回归（夹具评测 + 实库评测均通过）
- [ ] 不引入新的外部服务或联网测试

## 8. 风险与回退

| 风险 | 缓解 |
| --- | --- |
| 清别名后个别中文称呼不再解析 | 这些称呼若合法，应进入 `seed_aliases.json`（策展层）而不是靠 SCW 标签；清理前对比命中变化并抽查 |
| 夹具与真实数据形态偏离，测试通过而线上出错 | 保留实库冒烟与实库评测；夹具优先用真实同步函数写入 |
| CI 首次即红（历史格式问题、依赖实库的测试） | 先只加 build / vet / test；gofmt 另行处理；对必须依赖实库的用例显式标记 |
| 评测集扩容造成维护成本 | 要求“每个缺陷一题”；用 `tags` 分组，失败信息带题号与问句 |
| 其他来源写入通用别名 | 当前 SCW 写入侧已消除该路径；若其他来源出现，再按来源加守卫 |

回退：CI 可删除工作流文件；别名清洗可通过恢复写入逻辑并重跑 `sync scw` 还原；夹具与评测只增不改，不影响运行时。

## 9. 明确延后

| 项 | 去向 |
| --- | --- |
| Release 数据可复现（来源版本 / 校验值、失败回退上一版库） | 单独的发布工程文档 |
| 网络抓取统一重试 / 退避 / 限速、原始数据校验值 | 同步健壮性小任务 |
| MCP / CLI 契约测试（stdio 握手 + 调用） | 紧随本阶段，可复用夹具库 |
| `plan.go / answer.go` 体积与内部类型化 | [classification_maintainability_optimization.md](classification_maintainability_optimization.md) |
| `sc-search doctor` 一键自检 | 初始化体验优化 |
| 教程 / 客户端报错 / 赚钱路线等覆盖扩展 | [knowledge_coverage_optimization.md](knowledge_coverage_optimization.md) |

## 10. 建议开发流程

与此前阶段一致：

1. Cursor 按第 6 节实现（CI 与别名清洗可先行并行）；
2. Cursor Claude 复测：`go test ./...`、无实库环境验证、实库别名指标前后对比、MCP 抽问；
3. 通过后提交并推送。

开工入口：先加 `ci.yml` 并在 SCW 同步里去掉分类标签别名，再建夹具包迁移评测。
