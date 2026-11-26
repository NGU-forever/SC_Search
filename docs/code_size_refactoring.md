# 代码体积与结构重构设计

状态：已实施（阶段 0–9 完成；阶段 6 仅完成两处纯移动，`plan.go` 动作规则与类型化仍由分类文档推进）  
日期：2026-11-26  
优先级：P1（在工程保障之后、与分类维护性并行）  
范围：`go/internal` 下超大文件与超长函数的拆分、重复工具函数收敛、体积预算与回归锁定  
前置：[engineering_quality_optimization.md](engineering_quality_optimization.md)（CI / 夹具 / 评测集；本文依赖其中的行为锁定能力）  
关联：[classification_maintainability_optimization.md](classification_maintainability_optimization.md)（`plan.go` 的分类拆分与类型化在该文档，本文不重复，只规定与之衔接的顺序）、[answer_quality_optimization.md](answer_quality_optimization.md)

## 1. 背景

非测试 Go 代码约 1.25 万行。体量本身不大，问题在于**集中**：少数文件和函数承担了过多职责，修改任何一处都要在几百行里定位，也难以单独测试。

最近几轮改动（任务手续费投影、DetectAction、UEX 快照与 stale 措辞）都碰到了同一类成本：改动小，但落点在大文件的深处，只能靠全量测试兜底。

本阶段目标是**不改变任何外部行为**，把代码按职责拆小、把重复的小工具收敛到一处，并建立防止再次膨胀的体积预算。

## 2. 现状度量

数据来自 `go/internal` 非测试文件（2026-11-26）。

### 2.1 超大文件

| 文件 | 行数 | 主要内容 |
| --- | ---: | --- |
| `query/plan.go` | 843 | 分类正则、`QueryPlan`、实体解析、蓝图排序、浏览器策略 |
| `query/answer.go` | 787 | `AnswerQuestion`、Intent→Primary、事实挑选、置信度、草稿后缀 |
| `query/answer_paths.go` | 730 | 蓝图 / 商店 / 指南 / 计算四条回答路径 + 各自 SQL + 草稿格式化 |
| `orchestrate/present_missions.go` | 705 | 任务投影器、排名、字段、标题清洗、区间标题 |
| `ingest/scmdb.go` | 649 | 站点命名辅助、`ApplyScmdbPayload`、HTTP 下载、版本选择、Sync 入口 |
| `ingest/uex.go` | 566 | 系统推断、解析辅助、三段 apply、清残留、HTTP、Sync 入口 |

### 2.2 超长函数

| 函数 | 行数 | 问题 |
| --- | ---: | --- |
| `query.AnswerQuestion` | 388 | 一个函数里 `switch intent`，每个 case 几十行，混合查库、挑选、成文、置信度 |
| `mcp.NewServer` | 350 | 14 个工具的注册、handler 与公共闭包全在一个函数里 |
| `ingest.ApplyScmdbPayload` | 277 | 势力、蓝图池、蓝图、任务、地点、关系依次写入，局部 map 互相依赖 |
| `query.QueryPlan` | 222 | 见分类维护性文档 |
| `orchestrate.(missionProjector).Project` | 172 | 选行、排序、取字段、拼标题、拼草稿 |
| `query.EntityFacts` | 151 | 多段查询与组装 |
| `worker.tryNative` | 142 | 按工具名的大 `switch` |
| `orchestrate.buildBlocks` | 138 | 通用块 + 专用投影回退 + 工具级 switch |

### 2.3 重复的小工具

同一类“把 `any` 转成具体类型”的函数在各包各写一份，语义略有差异：

| 概念 | 现有实现位置 |
| --- | --- |
| 转字符串 | `query.str`、`ingest.strAny`、`worker.strAny`、`orchestrate.asString`、`router.asString` |
| 转 `[]map[string]any` | `query.castMaps/asMaps`、`orchestrate.asMaps` |
| 转 `[]string` | `orchestrate.asStringSlice`、`router.asStringSlice` |
| 转 bool / float / int64 | `orchestrate.asBool`、`query.asFloat`、`ingest.asInt64Any` |
| 取首个非空 | `ingest.firstNonEmpty(...string)`、`orchestrate.firstNonEmpty(row, keys...)`、`router.firstNonEmpty(a, b)` |
| 去重 | `orchestrate.uniqueStrings`、`orchestrate.appendUnique` |

同名但签名和行为不同的函数（尤其 `firstNonEmpty` 三个版本）是潜在的误用来源。

### 2.4 动态类型

`map[string]any` 在非测试代码中约 491 处（2026-11-26 初稿口径），集中在 `plan.go`（80）、`answer_paths.go`（49）、`answer.go`（48）、`mcp/server.go`（32）、`worker/executor.go`（30）。B2 完成后实测为 654 处 / 555 行（B2 前为 630 处 / 535 行；+24 处全部来自本阶段要求的 `anyx` 签名与 `answerContext` / `answerResult` 字段）。MCP 边界使用动态 JSON 是合理的；内部扩散的问题归分类维护性文档处理，本文只规定**不再新增**。

## 3. 目标与非目标

### 3.1 目标

1. 外部行为零变化：MCP 契约、`sc_ask` 回答、黄金评测、实库抽问结果保持一致。
2. 非测试文件不超过 500 行（有书面理由的例外除外）；单个函数不超过 120 行。
3. `AnswerQuestion`、`NewServer`、`ApplyScmdbPayload`、`Project`、`tryNative` 改为“入口 + 按职责拆分的小单元”。
4. 把重复的类型转换函数收敛到一个叶子包，消除同名异义的 `firstNonEmpty`。
5. 建立体积预算与报告脚本，防止回弹。
6. 每一步可独立评审、独立回滚。

### 3.2 非目标

- 不改变业务规则、排序、措辞、置信度算法。
- 不在拆分的同时修缺陷或调优；发现问题另开修复，先拆后改。
- 不引入新的依赖、框架或代码生成。
- 不一次性消灭所有 `map[string]any`（见分类维护性文档的类型化阶段）。
- 不追求“文件越小越好”：以职责内聚为准，不为凑行数而拆。

## 4. 方案

### 4.1 总原则：先搬家，后重构

每个拆分分两个独立提交：

1. **纯移动**：把函数原样剪切到新文件，不改一个字符（包名、导入除外）。`git diff -M` 应显示为重命名/移动，评审成本极低。
2. **必要的结构调整**：仅在移动完成且测试全绿之后，再做函数内拆分。

这样出问题时能快速判断是“移动错了”还是“重构错了”。

### 4.2 行为锁定（拆分前必须就位）

拆分的前提是能证明“没变”。现有 20 题黄金评测不足以覆盖，因此先加两层保护：

1. **特征快照（characterization snapshot）**
   - 对一批问句（建议 80–120 条，覆盖每个 intent、每条回答路径、stale、no_match、门禁）运行 `QueryPlan`、`AnswerQuestion`、`orchestrate.Ask`；
   - 把输出规范化（键排序、去掉耗时 trace 与时间戳）后写入 `testdata/snapshots/*.json`；
   - 重构期间 `go test` 比对快照，**不允许 diff**；确需变化时单独提交并说明原因。
2. **夹具库**
   - 快照在 [engineering_quality_optimization.md](engineering_quality_optimization.md) 4.2 的 `testfixture` 库上生成，CI 可运行，不依赖本机实库；
   - 实库快照仅作本地补充，不进 CI。

没有快照就不开始拆 `answer.go` / `plan.go` / `present_missions.go`。纯 ingest 和 mcp 的拆分可以先行，因为它们已有写库测试与可做 stdio 契约测试（见 4.7）。

### 4.3 共享类型转换包

新增叶子包 `go/internal/anyx`（不依赖任何业务包，避免循环依赖）：

```go
package anyx

func String(v any) string                      // 非字符串返回 ""
func Bool(v any) bool
func Float(v any) float64
func Int64(v any) int64
func Maps(v any) []map[string]any              // 兼容 []map[string]any 与 []any
func StringMap(v any) map[string]any
func Slice(v any) []any
func Strings(v any) []string
func First(vals ...string) string              // 取首个非空字符串
func FirstField(row map[string]any, keys ...string) string
func Unique(xs []string) []string
func AppendUnique(xs []string, s string) []string
```

迁移规则：

1. **先做等价性盘点**：对每一组同名函数写对照测试，列出边界输入（`nil`、数字、`[]byte`、嵌套）下各版本的返回值。**语义不同的不强行合并**，保留带后缀的专用函数（例如 `FirstField` 与 `First` 分开）。
2. 各包原函数先改为对 `anyx` 的一行转发，保证调用点零改动；全绿后再逐步把调用点替换，最后删除转发。
3. 同名异义的 `firstNonEmpty` 三个版本是首要消除对象：按签名分别映射到 `First`、`FirstField` 和调用处直接展开的 `a != "" ? a : b`。

### 4.4 `query` 包拆分（`answer*.go`）

#### 4.4.1 `AnswerQuestion` 改为分发表

现状：函数开头做一次公共准备（级别、计划、实体、`primary`、限额、`live`、`label`），随后按 Intent 在 `switch` 中各写几十行。

目标结构：

```go
type answerContext struct {
    DB          *sql.DB
    Question    string
    Level       string
    Plan        map[string]any
    Intent      string
    Path        string
    Entities    []map[string]any
    Primary     string
    PrimaryKind string
    Label       string
    Live        string
    FactLimit   int
    PriceLimit  int
    DocLimit    int
}

type answerResult struct { // 先与现有 map 输出一一对应，最后由 answerOut 转 map
    Draft      string
    Evidence   map[string]any
    Missing    []string
    FollowUps  []string
    Detail     bool
    Browser    map[string]any
}

type answerHandler func(*answerContext) (answerResult, error)

var answerHandlers = map[string]answerHandler{
    "facts":                     answerFacts,
    "prices":                    answerPrices,
    "mission_list":              answerMissions,
    "mission_money":             answerMissions,
    "docs":                      answerDocs,
    "live_web":                  answerLiveWeb,
    "live_local":                answerLiveLocal,
    "data_sync":                 answerDataSync,
}
```

`AnswerQuestion` 缩为：构造 `answerContext` → 先处理“推迟到 Route / 蓝图获取 / 商店获取 / 指南 / 计算”这几条前置分支 → 查 `answerHandlers` → 套用统一的置信度与软确认逻辑 → `answerOut`。

注意：当前 `switch` 里个别 case 共享局部变量（`draft`、`missing`、`followUps`、`detailAvailable`）。拆分时必须显式把它们收进 `answerResult`，不能引入包级可变状态。

#### 4.4.2 文件边界

```text
go/internal/query/
├── answer.go               # AnswerQuestion 入口、前置分支、分发、answerOut（目标 ≤ 200 行）
├── answer_context.go       # answerContext / answerResult 与构造
├── answer_facts.go         # facts 分支 + pickFacts + labelFactKey + factValue
├── answer_prices.go        # prices 分支 + slimPrices + priceStaleSuffix + draftLiveSuffix
├── answer_missions.go      # mission_list / mission_money 分支
├── answer_docs.go          # docs 分支
├── answer_live.go          # live_web / live_local / data_sync 分支
├── answer_confidence.go    # recomputeConfidence / needsSoftVerify
├── answer_blueprint.go     # answerBlueprintObtain + recipeMaterials + missionsForPool + 草稿
├── answer_shop.go          # answerShopObtain + shipBuyPrices + 草稿
├── answer_guide.go         # answerGuideIntent + 草稿
├── answer_calc.go          # answerCalcIntent
└── answer_format.go        # formatIntComma / reverseString / formatGuideLinks 等纯格式化
```

`SuggestedPrimary`、`ClassificationContext`、`IntentToSuggestedPrimary` 属于分类职责，随分类维护性文档的阶段 4 迁入 `classification.go`，本阶段**只做纯移动**，不改逻辑。

#### 4.4.3 与分类拆分的衔接

`plan.go` 的拆分（动作规则、Plan Builder、实体上下文）完全按 [classification_maintainability_optimization.md](classification_maintainability_optimization.md) 阶段 2–3 执行。本文补充两点边界约定：

1. `resolveEntities`、`entitiesEmbeddedInQuestion`、蓝图排序、挖矿实体过滤这一块（约 400 行）移入 `entity_context.go`，它与动作识别没有相互依赖；
2. 浏览器策略构造（`browserOff / browserErkul / browserMining / guidePlan`）移入 `plan_browser.go`。

两项都是纯移动，可以先于分类规则抽取完成，让后续分类拆分的 diff 更小。

### 4.5 `orchestrate/present_missions.go` 拆分

按职责切成四个文件，`Project` 缩为“编排四步”：

```text
present_missions.go          # missionProjector 与 Project 编排（选行 → 排名 → 取字段 → 成文）
present_missions_fields.go   # missionWantedFields / missionField / missionFieldLabel / firstMissionField
present_missions_rank.go     # rankMissionRows / missionQueryTokens / 排序辅助 / mixMissionDisplay
present_missions_headline.go # feeHeadline / rewardHeadline / numericRangeHeadline / 标题清洗
```

`Project` 内部拆成：

```go
rows     := selectMissionRows(st)          // 从 Evidence 取行并去重
ranked   := rankMissionRows(rows, st, wanted)
display  := mixMissionDisplay(...)
facts    := buildMissionFacts(display, wanted)
headline := buildMissionHeadline(...)
draft    := buildMissionDraft(...)
```

每个小函数输入输出明确，可以各自写表驱动测试，不再只能整体断言。`buildBlocks` 同理，把“通用块”“专用投影回退”“工具级 switch”拆成三个函数。

### 4.6 `ingest` 拆分

#### 4.6.1 `scmdb.go`

```text
scmdb.go           # 常量、Sync 入口（SyncScmdbPayload / SyncScmdbLive）
scmdb_apply.go     # ApplyScmdbPayload 编排
scmdb_phases.go    # 各阶段：factions / blueprintPools / blueprints / missions / locations
scmdb_missions.go  # enemyShipCounts / 站点命名 / 区域与难度标签
scmdb_fetch.go     # scmdbGetJSON / resolveScmdbVersion
```

`ApplyScmdbPayload` 的局部 map（`factionIDs`、`poolIDs` 等）收进一个小结构：

```go
type scmdbApply struct {
    conn       DBTX
    runID      int64
    version    string
    factionIDs map[string]int64
    poolIDs    map[string]int64
    // ...
    count      int
}
func (a *scmdbApply) factions(payload map[string]any) error
func (a *scmdbApply) blueprintPools(payload map[string]any) error
func (a *scmdbApply) missions(payload map[string]any) error
```

`ApplyScmdbPayload` 只负责先清、依序调用各阶段、汇总计数。阶段之间的依赖（例如任务依赖势力 ID）由结构体字段显式传递。库 P0 的事务边界不受影响，仍由调用方 `applyInTx` 包裹。

#### 4.6.2 `uex.go`

```text
uex.go          # 常量、UexPayload、Sync 入口
uex_apply.go    # ApplyUex / applyUexPayload / applyUexVehiclePurchases / pruneUexPrices
uex_fetch.go    # token 加载 / uexGet
uex_infer.go    # inferSystemFromTerminalName / 解析辅助（maybeFloat / asIntID / pickGameVersion）
```

`localization.go`、`scw.go` 体量尚可，本阶段只在触及时顺手拆；不为此单独开工。

### 4.7 `mcp/server.go` 拆分

`NewServer` 现为 350 行、14 个工具。改为：

```go
type serverDeps struct {
    db *sql.DB
}

func (d serverDeps) envResult(...)    // 现有公共闭包变成方法
func (d serverDeps) requireDB() ...
func (d serverDeps) withFreshness(rows []map[string]any) map[string]any

func NewServer(db *sql.DB) *mcp.Server {
    s := mcp.NewServer(...)
    d := serverDeps{db: db}
    registerEntityTools(s, d)       // sc_resolve_name / sc_entity_facts / sc_search_docs
    registerPriceTools(s, d)        // sc_best_prices / sc_terminal_prices
    registerMissionTools(s, d)      // sc_list_missions / sc_locations
    registerMetaTools(s, d)         // sc_data_freshness / sc_intents
    registerOrchestrationTools(s, d)// sc_ask / sc_route / sc_worker_run / sc_answer / sc_query_plan
    return s
}
```

对应文件：`tools_entities.go`、`tools_prices.go`、`tools_missions.go`、`tools_meta.go`、`tools_orchestration.go`，输入输出类型保留在 `types.go`。

`internal/mcp` 目前没有测试文件。拆分前先补一组**stdio 契约测试**：启动服务，列出工具，逐个调用并断言返回信封结构（`ok / data / meta`）、`stale` 元数据存在、错误路径返回 `isError`。这既是拆分的行为锁定，也补上了现有空白。

### 4.8 `worker.tryNative` 表驱动

现有按工具名的大 `switch`（142 行）改为：

```go
type nativeFn func(db *sql.DB, args map[string]any) (map[string]any, bool)

var nativeTools = map[string]nativeFn{
    "sc_resolve_name":     nativeResolveName,
    "sc_best_prices":      nativeBestPrices,
    "sc_terminal_prices":  nativeTerminalPrices,
    // ...
}
```

`tryNative` 缩为查表调用。每个工具一个小函数，便于单测，也让“新增工具”的修改范围固定为注册表加一个函数。

### 4.9 体积预算与防回弹

新增 `scripts/size_report.sh`（或 Go 小程序），输出：

- 非测试文件行数前 N；
- 函数长度前 N；
- 超出预算的清单（文件 > 500 行、函数 > 120 行）。

落地方式分两步，避免首次即红：

1. **报告模式**：CI 里作为非阻塞步骤（`continue-on-error: true`）输出清单，让数字可见；
2. **棘轮模式**：记录当前基线到 `scripts/size_baseline.txt`（仅记录“已知超标项及其当前值”）。之后只要**出现新的超标项，或已知超标项变得更大**就失败；重构使其变小则允许并要求更新基线。随着拆分推进，基线只会缩小，最终清空。

例外必须写在基线文件里并附一行原因（例如 `sc_resolve_name` 这类逻辑上就是长表的常量定义）。

## 5. 实施顺序

阶段之间的依赖：行为锁定 → 各包拆分；`ingest` 与 `mcp` 可先行；`query` / `orchestrate` 等快照就位后进行。

| 阶段 | 内容 | 前置 | 风险 |
| --- | --- | --- | --- |
| 0 | CI 与夹具库就位（来自工程质量文档） | — | 低 |
| 1 | 特征快照：对 `QueryPlan` / `AnswerQuestion` / `Ask` 生成并提交快照 | 0 | 低 |
| 2 | `mcp`：补 stdio 契约测试；`NewServer` 纯移动再拆分 | 0 | 低 |
| 3 | `ingest`：`scmdb` / `uex` 纯移动，再拆阶段结构 | 0 | 低（已有写库测试与库 P0 回滚测试） |
| 4 | `anyx` 包：盘点对照测试 → 转发 → 替换调用点 | 1 | 中（语义差异） |
| 5 | `query`：`answer*.go` 纯移动 → `AnswerQuestion` 分发表 | 1 | 中 |
| 6 | `query`：`plan.go` 的 `entity_context.go` / `plan_browser.go` 纯移动，再接分类拆分 | 1、分类文档 | 中 |
| 7 | `orchestrate`：`present_missions` 拆文件与函数；`buildBlocks` 拆分 | 1 | 中 |
| 8 | `worker.tryNative` 表驱动 | 1 | 低 |
| 9 | 体积报告脚本：报告模式 → 棘轮模式 | 部分完成后 | 低 |

每个阶段：一个或多个小提交，提交信息写明“move only”或“refactor, no behavior change”，并附上快照测试通过的证据。

## 6. 测试与验收

### 6.1 回归保护

| 保护 | 说明 |
| --- | --- |
| 特征快照 | 80–120 问，规范化输出不得有 diff |
| 黄金评测 | 夹具评测 + 实库评测均通过 |
| stdio 契约测试 | 14 个工具信封结构与错误路径不变 |
| 写库测试 | 库 P0 的事务回滚、UEX 清残留、索引测试保持通过 |
| `anyx` 对照测试 | 每个被合并的函数，新旧实现在边界输入上结果一致 |

### 6.2 度量指标（拆分前后对比）

| 指标 | 现状 | 目标 |
| --- | --- | --- |
| 非测试文件 > 600 行 | 5 个 | 0 |
| 非测试文件 > 500 行 | 6 个 | 0（带理由的例外除外） |
| 函数 > 150 行 | 6 个 | 0 |
| 函数 > 120 行 | 9 个（另含 `ApplyLocalizationMaps` 133 行） | 0 |
| 同名异义 `firstNonEmpty` | 3 个 | 0 |
| 重复的类型转换函数 | ≈ 15 个 | 收敛到 `anyx`，包内仅保留语义不同的专用版 |
| `map[string]any` 总数 | ≈ 491 | 不增加（棘轮） |

### 6.3 完成定义

- [x] 特征快照（`fresh.json` / `stale.json`，零 diff）与 stdio 契约测试在 CI 通过，且拆分过程中从未被修改以“迁就”重构
- [x] `AnswerQuestion` ≤ 120 行（63 行），各 Intent 有独立处理函数；每个 Intent 的输出由特征快照逐条锁定（未另写逐 Intent 单测）
- [x] `NewServer` ≤ 60 行（14 行），工具按组注册
- [x] `ApplyScmdbPayload` ≤ 80 行（25 行），阶段函数各自可测
- [x] `anyx` 就位，三个 `firstNonEmpty` 已消除（0 处定义），边界输入有对照测试
- [x] 体积报告运行于 CI，棘轮基线（`scripts/size_baseline.txt`）已提交，仅保留 `QueryPlan` / `EntityFacts` / `ApplyLocalizationMaps` 三个已说明例外
- [x] `go test ./...`、黄金评测、MCP 抽问均无回归

## 7. 风险与回退

| 风险 | 缓解 |
| --- | --- |
| 拆分过程中悄悄改变行为 | 纯移动与重构分提交；特征快照零 diff；快照变化必须单独说明 |
| `answer` 各 case 共享局部变量，拆分后漏传 | 先把共享状态显式收进 `answerResult`，再移动；快照覆盖每个 intent |
| `anyx` 合并掩盖语义差异（如数字转字符串） | 先写对照测试；语义不同的不合并；转发期保持旧函数名 |
| 包循环依赖 | `anyx` 为叶子包；`testfixture` 与 `query` 的方向遵循工程质量文档的约定 |
| 与分类文档的拆分冲突 | 本文只做 `plan.go` 的两处纯移动；动作规则与 Plan Builder 只由分类文档推进，并约定先完成纯移动再抽规则 |
| 大范围移动造成评审困难 | 每个提交只动一个包的一类职责；用 `git diff -M` 评审 |
| 体积预算过严阻碍正常开发 | 棘轮只阻止“变大”；例外写入基线并附理由；先报告后阻断 |

回退：每个阶段独立提交，可单独 `revert`；纯移动提交回退无副作用；`anyx` 转发期可随时撤回到包内旧函数。

## 8. 明确延后

| 项 | 去向 |
| --- | --- |
| `plan.go` 动作规则、Plan Builder、`SuggestedPrimary` 统一 | [classification_maintainability_optimization.md](classification_maintainability_optimization.md) 阶段 2–4 |
| `QueryPlan` / `RoutePlan` / `RunResult` / `EvidenceItem` 类型化 | 同上，阶段 5 |
| `localization.go` / `scw.go` / `entities.go` 的进一步拆分 | 触及时顺手做，不单独立项 |
| 并发、性能调优 | 无证据，不做 |
| 引入依赖注入框架或代码生成 | 不做 |

## 9. 建议开发流程

1. 先落地 [engineering_quality_optimization.md](engineering_quality_optimization.md) 的 CI 与夹具库；
2. Cursor 按第 5 节顺序实现，从 `mcp` 与 `ingest`（风险最低）开始；
3. 每个阶段由 Cursor Claude 复测：`go test ./...`、快照零 diff、MCP 抽问对比拆分前后字节级输出；
4. 通过后逐阶段提交，不合并成一个大提交；推送前确认 CI 通过。

开工入口：补 `mcp` 的 stdio 契约测试并生成特征快照，然后做 `NewServer` 与 `scmdb` / `uex` 的纯移动拆分。
