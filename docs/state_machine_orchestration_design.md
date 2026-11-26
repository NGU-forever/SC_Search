# SC Search Go 状态机编排设计

状态：v1 已实现（`sc_ask` + `go/internal/orchestrate`）  
目标版本：状态机编排 v1  
范围：Go MCP 查询侧，不包含数据同步与浏览器自动化实现

## 1. 背景

当前查询流程已具备以下可执行组件：

- `query.AnswerQuestion`：简单事实、价格等问题的快捷入口。
- `router.ExpandDAG`：根据 `primary + slots` 展开工具依赖图。
- `worker.RunPlan`：分波执行 DAG、聚合证据并产生回退信号。
- `intent_tools.json`：意图、槽位、工具和路径的机器可读剧本。

但完整流程仍依赖 Agent 阅读 `AGENTS.md` 和 `ORCHESTRATION.md` 后自行决定：

- 走 `sc_answer` 还是 `sc_route → sc_worker_run`。
- 如何消费 `replan_suggested` 和 `slot_retry_suggested`。
- 是否允许浏览器，以及何时必须先征得用户确认。
- 如何显式呈现数据陈旧、字段缺失和无匹配结果。

不同模型对长文规则的遵从程度不同，导致相同 MCP 在不同宿主上的行为不一致。v1 将这些运行时规则下沉为 Go 状态机，并提供单一默认 MCP 入口 `sc_ask`。Markdown 继续作为开发者说明，但不再承担运行时编排职责。

## 2. 目标与非目标

### 2.1 目标

1. 弱模型只调用一次 `sc_ask(question=...)` 即可完成简单或复杂查询。
2. 简单问题保留 `sc_answer` 快捷路径，避免不必要的 DAG 执行。
3. 复杂问题在服务端完成路由、Worker 执行和最多一次受控回退。
4. 浏览器决策以结构化门禁返回，MCP 服务不主动打开网页。
5. 所有关键决策有可测试、可观察的执行轨迹。
6. 保留现有细粒度工具，兼容强 Agent、调试和旧客户端。
7. 保持单个 Go 二进制、SQLite 和 STDIO MCP 的发布方式，不引入内置 LLM、Python 或 Node 运行时。

### 2.2 非目标

- 不实现通用工作流 DSL、动态插件系统或 Go 版 LangGraph。
- 不在 v1 中持久化执行状态或实现跨进程恢复。
- 不让 MCP 服务直接控制宿主浏览器。
- 不在服务端调用外部模型 API 做分类或成文。
- 不删除 `sc_answer`、`sc_route`、`sc_worker_run` 等现有工具。
- 不在本次工作中完成 Erkul 社区配装数据入库。

## 3. 架构

新增内部包：

```text
go/internal/orchestrate/
├── ask.go          # 对外 Ask 入口与主循环
├── state.go        # State、Phase、Result、Trace 等类型
├── classify.go     # primary/slots 推断与旁路判定
├── policy.go       # 回退、陈旧数据、浏览器门禁等硬规则
├── present.go      # answer_draft / answer_blocks 组装
├── errors.go       # 稳定错误码
└── ask_test.go     # 状态迁移与端到端测试
```

数据流：

```text
用户原话
  │
  ▼
sc_ask
  │
  ▼
orchestrate.Ask
  ├─ Classify
  ├─ Answer（简单题）
  ├─ Route → Worker（复杂题）
  ├─ Retry（最多一次 replan 或 slot retry）
  ├─ Present
  └─ BrowserGate
  │
  ▼
结构化 AskResult
  ├─ answer_draft：可直接使用的中文草稿
  ├─ answer_blocks：事实、警告、版本和来源
  ├─ browser：宿主后续动作门禁
  └─ trace：compact 执行轨迹
```

## 4. 状态模型

### 4.1 Phase

```go
type Phase string

const (
    PhaseClassify  Phase = "classify"
    PhaseAnswer    Phase = "answer"
    PhaseRoute     Phase = "route"
    PhaseWorker    Phase = "worker"
    PhaseRetry     Phase = "retry"
    PhasePresent   Phase = "present"
    PhaseAwaitUser Phase = "await_user"
    PhaseDone      Phase = "done"
    PhaseFailed    Phase = "failed"
)
```

### 4.2 State

```go
type State struct {
    Question   string
    Primary    string
    Slots      map[string]any
    Detail     string
    Phase      Phase

    Answer     map[string]any
    Plan       map[string]any
    Worker     map[string]any
    Evidence   []map[string]any
    Meta       map[string]any
    Browser    BrowserGate

    ReplanCount    int
    SlotRetryCount int
    Trace          []TraceEvent
}
```

状态只保存本次执行所需数据，不持有数据库连接、MCP Request 或宿主对象。依赖通过 `Engine` 注入。

### 4.3 Engine

```go
type Engine struct {
    DB      *sql.DB
    Invoker worker.Invoker
    Policy  Policy
}

func (e *Engine) Ask(ctx context.Context, in AskInput) AskResult
```

v1 使用固定节点表和受限循环，不引入动态节点注册：

```go
for !terminal(state.Phase) {
    if steps >= policy.MaxSteps {
        fail(ErrStepLimit)
        break
    }
    next, err := engine.runPhase(ctx, &state)
    recordTrace(...)
    state.Phase = next
}
```

`MaxSteps` 是防御性上限，不替代各类回退计数。建议 v1 固定为 8。

## 5. 状态迁移

```text
Classify
  ├─ 可旁路 ───────────────► Answer
  └─ 复杂/显式 primary ────► Route

Answer
  ├─ route_required ───────► Route
  ├─ 正常结果 ─────────────► Present
  └─ 异常 ─────────────────► Failed

Route
  ├─ 正常计划 ─────────────► Worker
  └─ 非法输入/计划异常 ────► Failed

Worker
  ├─ replan_suggested ─────► Retry
  ├─ slot_retry_suggested ─► Retry
  ├─ 可用证据 ─────────────► Present
  └─ 工具级异常 ───────────► Present（带失败警告）

Retry
  ├─ 允许且未达上限 ───────► Route
  └─ 已达上限 ─────────────► Present（标记 retry_exhausted）

Present
  ├─ browser.wait_confirm ─► AwaitUser
  └─ 无需浏览器 ───────────► Done

AwaitUser ─────────────────► 作为本轮终态返回
Failed ────────────────────► 作为本轮终态返回
```

`AwaitUser` 不在 MCP 进程中阻塞。它表示本轮返回 `status=awaiting_user`，由宿主询问用户。v1 不提供恢复令牌；用户确认后的浏览器操作仍由宿主执行。

## 6. 分类与旁路策略

### 6.1 输入优先级

1. 用户显式传入的合法 `primary`。
2. `query.QueryPlan` / `AnswerQuestion` 现有意图分类。
3. 本地实体解析结果与实体类型。
4. 无法可靠判断时使用 `other`，不得强猜。

用户传入 slots 必须经过 `router.ValidateRoute` 清洗。未知槽位丢弃，空字符串和 `nil` 丢弃。

### 6.2 旁路条件

满足以下条件时先调用 `query.AnswerQuestion`：

- 单实体静态事实。
- 单货物价格。
- 本地文档可直接回答的简单问题。
- 未显式要求复杂任务机制、比较、路线、改装计算或多槽约束。

以下情况直接进入 Route：

- 用户显式传入合法 `primary` 且意图属于复杂剧本。
- `AnswerQuestion` 返回 `route_required=true`。
- 问题要求手续费、冷却、站点池、声望、任务多条件或完整贸易路线。
- 需要 `loadout`、`mining` 等受浏览器门禁约束的工作流。

旁路判定由代码维护，不要求客户端模型判断。

## 7. Route 与 Worker 复用

状态机不重写现有路由与 Worker：

- Route 节点调用 `router.ExpandDAG(question, primary, slots, replanCount)`。
- Worker 节点调用 `worker.RunPlan(ctx, plan, worker.NewNativeInvoker(db))`。

v1 可以在内部将 `router.RoutePlan` 转成 `map[string]any` 以复用现有 Worker，但后续应将 `RunPlan` 增加类型化入口，减少内部 `map[string]any` 转换。MCP 边界可以继续接受动态 JSON，内部边界优先使用 Go 类型。

## 8. 回退策略

回退在服务端执行，不再把信号交给弱模型记忆。

### 8.1 上限

- `replan`：最多 1 次。
- `slot_retry`：最多 1 次。
- 单轮最多执行一种回退；`mismatch` 优先于 slot retry。
- 回退后仍失败则进入 Present，不形成循环。

### 8.2 Replan

触发条件：

```text
worker.replan_suggested == true
```

动作：

1. 根据现有 `suggested_primary`、实体类型和本地分类重新选择 primary。
2. 保留与新 playbook 兼容的 slots。
3. `ReplanCount++`。
4. 回到 Route。

若没有可靠的新 primary，则不重跑，直接以 `retry_exhausted` 警告进入 Present。

### 8.3 Slot retry

触发条件：

```text
worker.slot_retry_suggested == true
worker.replan_suggested != true
```

动作：

1. 优先使用 resolve 结果的英文 canonical name 更新对应槽位。
2. 若无 canonical，使用实体 hint 或标准化后的原查询词。
3. 不把“开荒、赚钱、萌新”等元话语写入 `rank_keyword` 或 `activity`。
4. `SlotRetryCount++`。
5. 回到 Route。

若无法产生与原值不同的新检索词，则不空跑，直接进入 Present。

## 9. 浏览器门禁

MCP 服务只返回结构化策略，不主动打开浏览器：

```go
type BrowserGate struct {
    Needed        bool             `json:"needed"`
    WaitConfirm   bool             `json:"wait_confirm"`
    LeaveOpen     bool             `json:"leave_open"`
    HandoffToUser bool             `json:"handoff_to_user"`
    Site          string           `json:"site,omitempty"`
    StartURL      string           `json:"start_url,omitempty"`
    SearchTerm    string           `json:"search_term,omitempty"`
    Resources     []map[string]any `json:"resources,omitempty"`
}
```

硬规则：

- `path=mcp_only`：`Needed=false`。
- `mcp_then_browser`：默认 `WaitConfirm=true`。
- `loadout`：先返回本地或社区主流方案；只有用户确认后宿主才打开 Erkul。
- `mining`：先返回静态证据；用户确认后再打开采矿工具。
- `search_term` 使用 resolve 后的英文标准名，不使用中文俗称。
- `leave_open=true` 时，宿主摘要后保留页面。
- MCP 返回 `AwaitUser` 不等于已经获得浏览器权限。

## 10. Present 结果

### 10.1 对外结构

```go
type AskResult struct {
    OK           bool              `json:"ok"`
    Status       string            `json:"status"`
    Primary      string            `json:"primary"`
    Slots        map[string]any    `json:"slots"`
    Path         string            `json:"path"`
    AnswerDraft  string            `json:"answer_draft"`
    AnswerBlocks AnswerBlocks      `json:"answer_blocks"`
    Evidence     []map[string]any  `json:"evidence,omitempty"`
    Meta         map[string]any    `json:"meta"`
    Browser      BrowserGate       `json:"browser"`
    FollowUps    []string          `json:"follow_ups,omitempty"`
    Trace        TraceSummary      `json:"trace"`
    Error        *AskError         `json:"error,omitempty"`
}
```

`status` 仅使用：

- `completed`
- `awaiting_user`
- `partial`
- `failed`

### 10.2 AnswerBlocks

```go
type AnswerBlocks struct {
    Headline string          `json:"headline"`
    Facts    []FactBlock     `json:"facts"`
    Warnings []string        `json:"warnings,omitempty"`
    Sources  []string        `json:"sources,omitempty"`
    Version  string          `json:"version,omitempty"`
    Missing  []string        `json:"missing,omitempty"`
}
```

客户端可以直接使用 `answer_draft`，也可以根据 `answer_blocks` 自定义渲染。数值、店名、任务名、材料和版本只能来自 Evidence/Meta。

### 10.3 强制表述策略

- `meta.stale=true`：`Warnings` 和 `answer_draft` 必须明确说明数据陈旧，并列出 `stale_sources`。
- 无字段：写入 `Missing`，草稿使用“本地未收录”，不得补全猜测值。
- `evidence_empty=true`：不得输出事实性结论；返回无匹配说明和近似命中。
- 部分工具失败但仍有强证据：`status=partial`，保留可验证结论并说明失败部分。
- `mcp_only` 且证据可用：不得建议外部浏览器核实。

## 11. MCP 接口

新增输入：

```go
type AskIn struct {
    Question    string         `json:"question" jsonschema:"User question"`
    Primary     string         `json:"primary,omitempty" jsonschema:"Optional primary override"`
    Slots       map[string]any `json:"slots,omitempty" jsonschema:"Optional slot overrides"`
    SlotsJSON   string         `json:"slots_json,omitempty" jsonschema:"Legacy JSON string form"`
    DetailLevel string         `json:"detail_level,omitempty" jsonschema:"brief (default) or full"`
    TraceLevel  string         `json:"trace_level,omitempty" jsonschema:"none, compact (default), or full"`
}
```

新增工具：

```text
sc_ask
```

工具描述应明确：

```text
Default one-shot Star Citizen entry: classify, route, execute local evidence
tools, retry once when safe, and return a Chinese draft plus browser gate.
```

兼容策略：

- `sc_answer` 保持原语义。
- `sc_route` / `sc_worker_run` 保留用于调试和强 Agent。
- `sc_intents` 增加 `default_entry: sc_ask`。
- 旧客户端无需立刻迁移。

## 12. 错误模型

稳定错误码：

```go
type ErrorCode string

const (
    ErrInvalidInput    ErrorCode = "invalid_input"
    ErrDatabaseMissing ErrorCode = "database_missing"
    ErrNoMatch         ErrorCode = "no_match"
    ErrRouteFailed     ErrorCode = "route_failed"
    ErrToolFailed      ErrorCode = "tool_failed"
    ErrRetryExhausted  ErrorCode = "retry_exhausted"
    ErrStepLimit       ErrorCode = "step_limit"
    ErrInternal        ErrorCode = "internal"
)
```

`no_match` 是可预期业务结果，通常返回 `OK=true`、`status=partial`，不应作为 MCP transport error。数据库缺失、非法输入和内部异常返回 `OK=false`。

## 13. Trace 与可观察性

默认 compact trace：

```json
{
  "phases": ["classify", "route", "worker", "present"],
  "mode": "route_worker",
  "replan_count": 0,
  "slot_retry_count": 0,
  "duration_ms": 42
}
```

full trace 额外包含：

```json
{
  "phase": "worker",
  "decision": "slot_retry",
  "reason": "strong_family_missing",
  "duration_ms": 18
}
```

Trace 不包含密钥、环境变量、完整数据库路径或用户隐私数据。执行轨迹用于调试和评测，不作为玩家答案正文。

## 14. 测试策略

### 14.1 状态迁移单测

至少覆盖：

1. 简单 Carrack 参数：`Classify → Answer → Present → Done`。
2. 单货物价格：旁路且保留 stale 警告。
3. 多槽任务机制：`Answer(route_required) → Route → Worker → Present`。
4. mismatch：只 replan 一次。
5. strong family missing：只 slot retry 一次。
6. 无法产生新检索词：不重复执行相同计划。
7. `mcp_only`：BrowserGate 必须关闭。
8. `loadout`：返回主流方案和 `WaitConfirm=true`，不得视为已开 Erkul。
9. Worker 部分失败：返回 `partial` 而不是丢弃全部证据。
10. 完全无匹配：不得生成证据外事实。

### 14.2 MCP 契约测试

- `sc_ask` schema 只要求 `question`。
- `slots` 和 `slots_json` 均可用。
- `structuredContent` 始终为对象。
- 从仓库外 cwd 启动仍可调用。
- `answer_draft`、`meta`、`browser`、`trace` 始终存在。

### 14.3 黄金评测集

新增 `eval/questions.jsonl`，每条声明：

```json
{
  "question": "Carrack 载货量和厂家？",
  "expected_primary": "ship_stats",
  "must_use": ["sc_entity_facts"],
  "must_contain": ["456", "Anvil"],
  "must_not_browse": true
}
```

首批建议覆盖 20–50 条：中英文实体、俗称、价格 stale、多槽任务、无匹配、回退、改装门禁、蓝图获取和版本问题。

## 15. 单一真相源

运行时 playbook 继续以 `go/internal/router/intent_tools.json` 为权威。实施状态机后应消除 `intents_catalog.json` 与编排 Markdown 的手工重复：

1. `sc_intents` 直接从权威 playbook 生成目录。
2. 开发者文档中的意图表由脚本或测试快照生成。
3. CI 增加一致性测试，禁止两份可执行 playbook 漂移。

`ORCHESTRATION.md` 保留为开发者解释文档，但开头应注明“非运行时真相源”。`AGENTS.md` 的事实问答规则最终缩短为：

```text
事实题默认调用 sc_ask(question=用户原话)。
使用 answer_draft / answer_blocks 成文。
browser.wait_confirm=true 时先征得用户确认。
细粒度 route/worker 仅用于调试或显式覆盖。
```

初始化与 Release 配置规则仍保留在 `AGENTS.md`，因为它们发生在 MCP 可用之前，不能由 `sc_ask` 执行。

## 16. 实施阶段

### 阶段 1：最小状态机

- 新增 `internal/orchestrate` 类型和固定执行循环。
- 复用 `AnswerQuestion`、`ExpandDAG`、`RunPlan`。
- 实现 `sc_ask` MCP 工具。
- 实现 compact trace 和稳定错误码。

验收：简单题、复杂题和无匹配题均只需一次 `sc_ask` 调用。

### 阶段 2：硬策略下沉

- 将 stale、missing、browser gate、loadout/mining 门禁迁入 Policy/Presenter。
- 自动执行一次 replan 或 slot retry。
- 增加状态迁移和门禁测试。

验收：不再依赖客户端读取 `ORCHESTRATION.md` 才能正确重试和询问浏览器确认。

### 阶段 3：文档瘦身与评测

- 建立黄金评测集。
- `AGENTS.md` 改为短入口。
- `ORCHESTRATION.md` 改为开发说明并标注非真相源。
- 合并 playbook 重复来源。

验收：不同宿主只要能调用 `sc_ask`，工具选择和门禁决策保持一致。

### 阶段 4：按证据决定的后续优化

只有评测显示明确瓶颈后再选择：

- 社区主流配装定期入库。
- 类型化 Worker 输入，减少内部动态 map。
- 字段级来源与 stale。
- 跨轮状态持久化和恢复。
- 可选 LLM classifier。

## 17. 成功标准

v1 完成需满足：

1. 事实问答默认只需调用 `sc_ask`。
2. 现有 13 个工具继续可用，旧行为无破坏性变化。
3. 复杂题的 route/worker/retry 由服务端完成。
4. `loadout` 和 `mining` 未经确认不产生“已打开浏览器”的状态。
5. stale、missing、no-match 在结果结构和草稿中均可见。
6. 状态迁移、MCP 契约和现有 Go 测试全部通过。
7. Release 仍为单个 Go 二进制 + SQLite，不新增运行时依赖。
8. 至少 20 条黄金问题可以自动评测路由、证据和门禁。

## 18. 设计约束总结

- 状态机负责确定性控制；模型负责用户语言理解的最后润色。
- 策略进入 Go 代码；playbook 留在单一机器可读文件；Markdown 只解释。
- 证据不足时降低结论，不通过更多自由推理填空。
- 浏览器是宿主能力，MCP 只返回门禁与建议动作。
- 第一版保持小而可测，不预先实现持久化、通用 DSL 或内置模型。
