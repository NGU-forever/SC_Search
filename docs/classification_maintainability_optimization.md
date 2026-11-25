# `sc_ask` 分类与内部类型维护性优化设计

状态：待实施  
日期：2026-11-26  
优先级：P1（第二阶段）  
范围：QueryPlan 动作分类、Primary 映射、计划构建和 Worker 内部类型边界  
前置阶段：[answer_quality_optimization.md](answer_quality_optimization.md)  
后续阶段：[knowledge_coverage_optimization.md](knowledge_coverage_optimization.md)

## 1. 背景

当前规则分类已经通过 20 条严格黄金评测，并解决了动作意图被实体命中覆盖的问题。现阶段正确性可接受，但维护成本正在上升：

- `go/internal/query/plan.go` 已超过 800 行；
- 正则定义、动作优先级、实体过滤、计划构建和浏览器策略位于同一文件；
- `answer.go` 同时承担问答、Intent 映射和上下文 Primary 选择；
- Router/Worker/Orchestrate 之间仍大量传递 `map[string]any`；
- 新增问法时容易只补正例，缺少同类反例；
- 同一概念可能同时出现在 QueryPlan、SuggestedPrimary 和 replan policy 中。

本阶段目标不是更换分类算法，而是在保持行为不变的前提下建立清晰、可测试的边界。

## 2. 问题定义

### 2.1 分类职责混合

当前 `QueryPlan` 同时做：

1. 解析实体；
2. 识别用户动作；
3. 计算置信度；
4. 选择 MCP/browser path；
5. 构造工具步骤；
6. 填写 reason、stop_if 和 browser gate。

任何一项变化都可能影响整个函数。

### 2.2 规则优先级隐含在 if 顺序

例如：

```text
mining → loadout → mission_money → mission_list → prices → client…
```

优先级是业务语义的一部分，但当前主要由代码位置表达。移动代码块就可能改变分类结果，且没有统一的冲突解释。

### 2.3 上下文映射分散

`intent → primary` 可能经过：

- `QueryPlan`
- `SuggestedPrimary`
- `orchestrate.classify`
- `replanFromConflict`

这些层级各有合理职责，但缺少共享的类型和明确输入输出，容易出现首次分类和重规划语义漂移。

### 2.4 动态 map 扩大运行时风险

MCP 边界使用动态 JSON 是合理的，但内部继续大量使用 `map[string]any` 会带来：

- 字段拼写错误只能运行时发现；
- 数字类型在 `float64`、`int64`、`int` 间转换；
- Router 计划和 Worker 结果缺少编译期约束；
- 测试需要手工构造复杂 map。

## 3. 目标与非目标

### 3.1 目标

1. 保持现有分类行为和 MCP JSON 契约不变。
2. 将动作检测、实体上下文、计划构建分成独立单元。
3. 让规则优先级显式、可测试。
4. 每条动作规则必须有中文正例、英文正例和冲突反例。
5. 逐步把内部边界类型化，动态 map 仅保留在 MCP/JSON 边界。
6. 降低新增 Intent 和同义表达的修改范围。

### 3.2 非目标

- 不引入 LLM classifier、向量分类或第三方 NLP 框架。
- 不实现通用工作流 DSL。
- 不修改 `intent_tools.json` 的真相源地位。
- 不一次性重写所有 Query/Router/Worker。
- 不在没有评测证据时调整现有业务优先级。

## 4. 方案选择

### 4.1 方案 A：保持单文件，只增加注释和表驱动测试

优点：风险最低。  
缺点：职责和动态类型问题仍在，无法降低长期修改成本。

### 4.2 方案 B：分层分类器 + 渐进类型化（推荐）

```text
question
  → DetectAction
  → ResolveContext
  → SelectIntent
  → BuildQueryPlan
  → SuggestedPrimary
```

先拆纯函数和类型，不改变对外 API；随后只在 Router/Worker 的稳定边界引入类型。

优点：

- 可逐步迁移；
- 每一步都可通过现有黄金集验证行为不变；
- 不需要一次性改动 MCP 契约。

缺点：迁移期间会短暂存在 typed struct 与 map 的适配层。

### 4.3 方案 C：直接引入模型分类器

优点：自然语言覆盖可能更广。  
缺点：引入网络、成本、非确定性和隐私问题，不符合当前本地单二进制目标。

结论：采用方案 B。

## 5. 建议架构

### 5.1 Query 分类边界

```text
go/internal/query/
├── plan.go                 # QueryPlan 对外兼容入口
├── action.go               # Action 类型与 DetectAction
├── action_rules.go         # 规则定义和显式优先级
├── classification.go       # ClassificationContext / SelectIntent
├── plan_builders.go        # 按 Intent 构造 QueryPlan
├── entity_context.go       # 实体过滤和槽位上下文
└── *_test.go
```

建议类型：

```go
type Action string

const (
    ActionFacts        Action = "facts"
    ActionPrice        Action = "price"
    ActionLoadout      Action = "loadout"
    ActionMining       Action = "mining"
    ActionMissionMoney Action = "mission_money"
    ActionMissionRep   Action = "mission_rep"
)

type ActionMatch struct {
    Action     Action
    Confidence string
    RuleID     string
}

type ClassificationContext struct {
    Question string
    Action   ActionMatch
    Entities []ResolvedEntity
}
```

`RuleID` 只用于 trace 和测试，不对用户暴露内部正则。

### 5.2 显式规则优先级

规则采用有序表，而不是散落的 if：

```go
type ActionRule struct {
    ID       string
    Priority int
    Match    func(string) bool
    Action   Action
}
```

优先级原则：

1. 明确操作和计算动作高于实体事实；
2. 组合规则高于单关键词规则；
3. 用户明确问字段时，高于泛化领域词；
4. 无可靠动作时才退化到实体 facts 或 `other`。

例如，“采矿船货舱多大”应由静态参数反例阻止 `mining`；“采矿配激光”才进入 mining。

### 5.3 Plan Builder

每类计划由小型 builder 负责：

```go
type PlanBuilder func(PlanContext) QueryPlanResult
```

Builder 只负责：

- `intent`
- `path`
- `mcp_steps`
- `browser`
- `stop_if`
- `reason`

动作识别不直接拼 MCP 参数；实体解析不直接决定浏览器门禁。

### 5.4 Primary 映射

保留 `SuggestedPrimary(ctx ClassificationContext)` 作为唯一上下文化映射入口。

约束：

- `IntentToSuggestedPrimary(intent string)` 仅作为兼容包装；
- `orchestrate.classify` 不维护第二份 intent 映射表；
- replan 可以参考证据冲突，但最终仍调用同一上下文映射函数；
- Primary 合法集合继续由 Router playbook 校验。

### 5.5 渐进类型化边界

迁移顺序：

1. `ResolvedEntity`
2. `QueryPlanResult`
3. `router.RoutePlan`
4. `worker.RunResult`
5. `EvidenceItem`

MCP handler 在入口把 JSON 转成类型，在出口把类型编码为 JSON。内部不要求立即消灭所有 map。

建议不要先类型化原始第三方 Evidence data，因为不同工具返回结构不同。可先类型化公共信封：

```go
type EvidenceItem struct {
    ID   string
    Tool string
    OK   bool
    Data any
    Meta map[string]any
}
```

具体 `Data` 由对应 Query 工具定义类型。

## 6. 实施步骤

### 阶段 1：行为锁定

1. 将黄金集扩展到至少 50 条：
   - 每个 Action 至少中文、英文和冲突反例；
   - 覆盖同句多动作词；
   - 覆盖实体别名与动作词冲突。
2. 增加分类 trace 断言，记录 `rule_id`。
3. 运行全量测试建立迁移基线。

### 阶段 2：提取 Action Classifier

1. 从 `plan.go` 提取动作正则和组合判断。
2. 建立 `ActionRule` 有序表。
3. 保留 `QueryPlan` 函数签名。
4. 对照黄金集确保 Primary、path、browser 完全不变。

### 阶段 3：提取 Plan Builders

按以下顺序拆分：

1. loadout / mining
2. mission_money / mission_rep
3. trade / trade_route
4. guide 类 Intent
5. facts / other

每拆一组运行对应测试和全量黄金集。

### 阶段 4：统一 Primary 映射

1. 清点所有 intent → primary 入口。
2. 统一到 `SuggestedPrimary`。
3. replan 只提供新的上下文，不维护另一套静态映射。
4. 删除已无调用的兼容分支。

### 阶段 5：类型化稳定边界

1. 类型化 QueryPlan 返回值。
2. 为 RoutePlan 增加类型化入口，JSON map 入口保留兼容。
3. 为 Worker 增加类型化结果。
4. MCP handler 负责 map/JSON 适配。

不要求在本阶段类型化所有工具 `Data`。

## 7. 测试策略

### 7.1 Action Classifier

每个规则至少包括：

- 中文正例；
- 英文正例；
- 相邻 Intent 反例；
- 实体别名冲突反例；
- 否定或“只问参数”反例。

### 7.2 行为等价测试

重构前后必须保持：

- `primary`
- `path`
- `browser.needed`
- `browser.wait_confirm`
- 工具集合
- 最大重试次数

一致。

工具 DAG 的内部 node ID 不必固定，只断言语义。

### 7.3 类型契约测试

- JSON → typed plan → JSON 往返不丢字段；
- 未知槽位仍被清理；
- 缺字段返回稳定错误，不 panic；
- 数字字段不因类型转换变为字符串或丢失。

### 7.4 回归命令

```bash
cd go
go test ./internal/query ./internal/router ./internal/worker ./internal/orchestrate -count=1
go test ./... -count=1
```

## 8. 验收标准

1. 现有和扩充后的黄金评测全部通过。
2. `QueryPlan` 对外签名和 MCP JSON 契约保持兼容。
3. 动作规则优先级由明确数据结构和测试表达。
4. 新增一个动作同义词不需要修改 Router、Worker 和 Presenter。
5. 首次分类与 replan 不再维护冲突的 Primary 映射。
6. 至少 QueryPlan、RoutePlan、Worker 公共结果边界有 Go 类型。
7. `plan.go` 不再同时承载全部正则、实体过滤和计划构造。
8. 全量 Go 测试通过。

## 9. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 纯重构引入行为变化 | 先扩黄金集，每一步做行为等价验证 |
| 文件拆分过度 | 按业务职责拆，不按每个 Intent 建一个文件 |
| typed/map 双轨长期存在 | 明确迁移边界和兼容层删除条件 |
| RuleID 暴露实现细节 | 仅放 full trace 或测试，不放普通回答 |
| 类型化第三方数据成本过高 | 先类型化信封和稳定结果，不强制类型化所有 Data |

## 10. 完成定义

当分类行为保持稳定、规则优先级可直接测试、Primary 映射只有一个权威入口，并且内部核心边界获得编译期类型约束时，本阶段完成。
