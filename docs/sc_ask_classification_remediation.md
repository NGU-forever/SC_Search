# `sc_ask` 分类与评测修复设计

状态：已实现（严格黄金评测 20/20）  
日期：2026-11-26  
范围：Go MCP 查询侧的意图分类、Primary 映射、受控重规划与自动评测  
关联设计：[state_machine_orchestration_design.md](state_machine_orchestration_design.md)

## 1. 背景

`sc_ask` 已将简单旁路、Router/Worker、受控重试、成文和浏览器门禁收敛到 Go 状态机。现有 Go 单元测试全部通过，但对 `eval/questions.jsonl` 的真实用户问法进行端到端测试后，发现状态机能否进入正确路径高度依赖 `query.QueryPlan` 的首次分类。

本次测试覆盖 20 条黄金问题。临时评测最初报告 12/20 通过，但该评测在 `answer_bypass` 模式下放宽了 `expected_primary`，掩盖了部分错误。严格按黄金集字段判断时，结果约为 9/20 通过、11/20 失败。

主要失败示例：

| 问题 | 期望 Primary | 实际 Primary | 后果 |
| --- | --- | --- | --- |
| 流星改装方案 | `loadout` | `ship_stats` | 未返回浏览器确认门禁 |
| 怎么采矿配激光 | `mining` | `ship_stats` | “采矿”被解析成 ATLS GEO |
| Bit Zeros ship heist 手续费和冷却 | `mission_money` | `ship_stats` | Worker 无有效证据，重试耗尽 |
| 红风 Junior 能接什么任务 | `mission_rep` | `mission_money` | 丢失声望语义 |
| CCU 是什么 | `pledge_meta` | `other` | 未进入指南与确认流程 |
| Invictus 活动怎么玩 | `event_guide` | `other` | 未进入活动指南流程 |
| Carrack 在哪买 | `ship_buy` | `ship_stats` | 未进入购船信息流程 |
| 当前 Live 版本 | `patch_live` | `ship_stats` | 未返回版本信息 |
| 黄金在 Area18 什么价 | `trade` | `ship_stats` | “什么价”未命中价格意图 |

## 2. 问题定义

### 2.1 中文意图规则覆盖不足

`go/internal/query/plan.go` 中的规则未覆盖部分常见表达：

- `loadoutRE` 包含“配装”，不包含“改装”。
- `miningRE` 包含“激光头、采矿头”，不包含“采矿配激光”等自然表达。
- `shipShopRE` 仅覆盖“游戏里哪里买船、船店”等较窄表达。
- `eventRE` 未覆盖 `Invictus` 等活动名称。
- `liveRE` 未覆盖“当前 Live 版本、游戏版本”。
- `pricesRE` 未覆盖“什么价、售价、收购价”。

当动作意图没有命中时，查询会退化为实体事实查询。

### 2.2 实体命中被误当成静态参数意图

`QueryPlan` 会解析整句中的实体。当动作规则未命中但实体命中时，会返回 `facts`：

```text
用户问题
  → 实体解析成功
  → 动作意图未命中
  → facts
  → ship_stats
```

因此，“流星改装方案”虽然正确解析出 RSI Meteor，但最终被解释为查询船舶静态参数。“采矿”还可能命中 Argo ATLS GEO 的中文别名，进一步放大误判。

### 2.3 Intent 到 Primary 的映射丢失上下文

当前 `IntentToSuggestedPrimary(intent string)` 只接收一个字符串，无法参考实体类型或问题中的机制词，存在以下损失：

- 所有 `facts` 均映射为 `ship_stats`，包括武器、势力和无实体问题。
- `mission_list` 固定映射为 `mission_money`，不能区分声望任务。
- `pledge_meta`、`event_guide` 等没有显式映射，落入 `other`。
- 无法根据“手续费、冷却、Junior、声望”等上下文选择 Primary。

### 2.4 Retry 重复首次误判

重规划优先复用 `PlanIntent`。如果首次结果是错误的 `facts → ship_stats`，重试通常仍得到同一个 Primary，无法产生新检索路径，最终返回 `retry_exhausted`。

### 2.5 测试未覆盖真实入口

- 部分状态机测试通过显式 `Primary` 绕过分类器。
- `plan_test.go` 只覆盖少量简单问题。
- `eval/questions.jsonl` 没有仓库内正式、严格、可重复执行的评测器。
- 临时评测对 `answer_bypass` 放宽 Primary，导致假阳性。

## 3. 目标与非目标

### 3.1 目标

1. 20 条黄金问题严格通过，不允许通过旁路模式掩盖 Primary 错误。
2. 将“用户动作”和“实体解析”分开处理，动作优先决定查询路径。
3. 保留现有状态机、Router/Worker 和浏览器门禁结构。
4. Primary 推断能够参考 intent、实体类型和问题上下文。
5. 重规划必须能够修正证据类型与 Primary 的明显冲突。
6. 不引入外部模型或新的运行时依赖。

### 3.2 非目标

- 不引入通用 NLP 框架或内置 LLM 分类器。
- 不重写 Router/Worker DAG。
- 不改变 MCP 对外工具名称或 `sc_ask` 请求结构。
- 不让 MCP 服务主动打开浏览器。
- 不在本次修复中扩充外部数据源。

## 4. 方案选择

### 4.1 方案 A：仅扩充现有正则

直接为现有规则增加“改装、采矿、什么价、Invictus”等关键词。

优点：

- 改动最小。
- 可以快速修复部分黄金问题。

缺点：

- `facts → ship_stats` 和 `mission_list → mission_money` 的结构性问题仍存在。
- 后续新增问法会继续堆积在大正则中。
- 容易产生“采矿船货舱”被误判为 mining 等过宽匹配。

### 4.2 方案 B：动作意图与实体解析分层

先识别问题中的动作和机制，再用实体解析填充槽位；只有动作不明确时才回退 facts。Primary 推断同时接收 intent、实体和原始问题。

优点：

- 修复根因，仍保持确定性和无外部依赖。
- 可以用独立测试验证动作识别和实体填槽。
- 能处理“同一实体，不同动作”的问题。

缺点：

- 需要调整 `QueryPlan` 和 Primary 映射接口。
- 需要补充较完整的回归测试。

### 4.3 方案 C：引入模型分类器

使用外部模型直接返回 intent 和 slots。

优点：

- 自然语言覆盖面更大。

缺点：

- 破坏离线、确定性和单二进制约束。
- 增加延迟、费用、鉴权和不可重复性。
- 与当前状态机 v1 的设计目标冲突。

### 4.4 结论

采用方案 B，并吸收方案 A 中必要的关键词补充。方案 C 不进入本次范围。

## 5. 目标架构

### 5.1 分类流程

```text
用户原话
  │
  ├─ ① Action Classifier
  │     识别 loadout / mining / trade / ship_buy /
  │     mission_rep / mission_money / event / pledge / live 等
  │
  ├─ ② Entity Resolver
  │     解析 ship / commodity / faction / mission / item
  │
  ├─ ③ Slot Binder
  │     根据 Action + Entity kind 填充合法槽位
  │
  ├─ ④ Primary Selector
  │     综合 intent + entities + question 选择 Primary
  │
  └─ ⑤ Bypass / Route 决策
```

动作分类优先于实体 facts 回退，但实体解析仍可先执行以提供类型信息。关键约束是：实体命中不能单独证明用户在查询静态参数。

### 5.2 动作识别规则

规则应表达“上下文组合”，避免只匹配单个宽泛词：

| 动作 | 建议条件 |
| --- | --- |
| `loadout` | `改装、配装、loadout、Erkul`，或船实体配合火力/护盾配置语义 |
| `mining` | `采矿、挖矿、mining` 与 `激光、模块、配装、破矿、阻力` 组合，或明确“采矿头/激光头” |
| `ship_buy` | 船实体与 `哪里买、在哪买、船店、租船` 组合 |
| `trade` | 商品/终端与 `什么价、售价、收购价、最贵、最便宜、买入、卖出` 组合 |
| `mission_rep` | 势力与 `声望、等级、Junior、能接什么任务` 组合 |
| `mission_money` | 任务实体或任务语义与 `手续费、冷却、奖励、站点、刷法` 组合 |
| `pledge_meta` | `CCU、Warbond、融船、Store Credit、LTI` |
| `event_guide` | `Invictus、无敌舰队周、IAE、免费试飞` 等活动名称或活动玩法语义 |
| `patch_live` | `当前 Live 版本、游戏版本、最新补丁、Patch Notes` |

反例必须进入测试：

- “采矿船货舱多大”应保持静态参数查询。
- “Carrack 火力多大”不应自动要求打开 Erkul。
- “黄金是什么”不应进入价格查询。

### 5.3 Primary 推断接口

将只接收 intent 的接口调整为接收完整分类上下文，例如：

```go
type ClassificationContext struct {
    Intent   string
    Question string
    Entities []map[string]any
}

func SuggestedPrimary(ctx ClassificationContext) string
```

最低映射要求：

| 条件 | Primary |
| --- | --- |
| `facts` + 第一实体为 ship | `ship_stats` |
| `facts` + 第一实体非 ship | `other` |
| `mission_list` + 声望/等级语义 | `mission_rep` |
| `mission_list` + 收益/机制语义 | `mission_money` |
| `pledge_meta` | `pledge_meta` |
| `event_guide` | `event_guide` |
| `ship_shop` | `ship_buy` |
| `live_web` 或本地版本查询 | `patch_live` |

为保持兼容，可暂时保留 `IntentToSuggestedPrimary(string)`，内部调用新接口的无上下文降级路径；新状态机入口必须使用完整上下文接口。

### 5.4 受控重规划

重规划除 `PlanIntent` 外，还应检查证据与 Primary 是否冲突：

- Primary 为 `ship_stats`，问题含任务机制词且无 ship 证据：候选 `mission_money`。
- Primary 为 `mission_money`，实体为 faction 且问题含等级/声望词：候选 `mission_rep`。
- Primary 为 `other`，问题明确命中 pledge/event/live：切换到对应 Primary。

重规划仍最多执行一次。无法形成可靠新 Primary 时保持当前失败语义，不做猜测式切换。

## 6. 实施分解

### 阶段 1：建立严格评测基线

1. 新增 Go 评测测试，逐行读取 `eval/questions.jsonl`。
2. 严格检查所有已声明字段。
3. `expected_primary` 不因 `answer_bypass` 放宽。
4. 将当前失败稳定复现为测试失败。

建议位置：

```text
go/internal/orchestrate/eval_test.go
```

### 阶段 2：扩充 QueryPlan 动作分类

1. 将动作判断提取为可独立测试的函数。
2. 补齐中文和英文常见表达。
3. 使用组合条件避免宽泛关键词误判。
4. 为每种动作增加正例和反例。

主要文件：

```text
go/internal/query/plan.go
go/internal/query/plan_test.go
```

### 阶段 3：上下文化 Primary 映射

1. 新增 `ClassificationContext`。
2. 补齐 pledge、event、live 等映射。
3. 根据实体 kind 处理 facts。
4. 根据机制词区分 mission_rep 和 mission_money。
5. 更新 `orchestrate.classify` 使用新接口。

主要文件：

```text
go/internal/query/answer.go
go/internal/orchestrate/classify.go
```

### 阶段 4：修复重规划

1. 在 `replanPrimary` 中加入证据类型和问题上下文。
2. 保留最多一次重规划限制。
3. 增加错误 Primary 被纠正和不可纠正两类测试。

主要文件：

```text
go/internal/orchestrate/policy.go
go/internal/orchestrate/ask_test.go
```

### 阶段 5：端到端验证

1. 运行所有 Go 测试。
2. 严格运行 20 条黄金问题。
3. 重新构建 `go/sc-search`。
4. 通过 MCP 实际调用验证旁路、Router/Worker、浏览器确认、无匹配和重试路径。

## 7. 测试策略

### 7.1 单元测试

- Action Classifier：每个动作至少一个中文正例、英文正例和反例。
- Primary Selector：覆盖实体 kind 和上下文组合。
- Slot Binder：验证 ship、commodity、faction、mission 不交叉污染。
- Retry Policy：验证只重试一次且必须产生新 Primary 或新检索词。
- Browser Gate：继续验证 `mcp_only` 关闭浏览器，loadout/mining 必须 `wait_confirm`。

### 7.2 黄金评测

黄金集每条记录按声明字段严格验证：

```text
expected_primary
must_contain
must_not_contain
must_not_browse
must_wait_confirm
```

此外增加通用断言：

- `status=failed` 必须有稳定错误码。
- `status=partial` 必须有明确 missing/warning。
- `must_not_browse=true` 时 `browser.needed` 必须为 false。
- `must_wait_confirm=true` 时 `browser.needed` 和 `browser.wait_confirm` 必须同时为 true。
- 输出不得包含黄金集禁止的编造内容。

### 7.3 MCP 冒烟测试

至少实际调用以下问题：

| 问题 | 预期路径 |
| --- | --- |
| Carrack 载货量和厂家？ | `answer_bypass`，无浏览器 |
| 黄金哪里卖最贵 | `answer_bypass`，带陈旧数据提示 |
| 流星改装方案 | `route_worker` 或相应复杂路径，`awaiting_user` |
| 怎么采矿配激光 | mining，`wait_confirm=true` |
| Bit Zeros ship heist 手续费和冷却 | mission_money，不能要求客户端改用 `sc_route` |
| zzz_not_a_real_sc_entity_qqq 参数 | 无匹配且不编造 |

## 8. 验收标准

1. `go test ./... -count=1` 全部通过。
2. `eval/questions.jsonl` 20/20 严格通过。
3. 新增反例全部通过，未因扩大规则造成明显误判。
4. `sc_ask` 对简单问题保持旁路，不产生不必要 DAG。
5. loadout/mining 未经用户确认不得表现为已打开浏览器。
6. 复杂任务题在服务端完成 route/worker/retry，不把编排责任退回客户端。
7. 无匹配、字段缺失和陈旧数据继续显式呈现。
8. 发布形态仍为单个 Go 二进制、SQLite 和 STDIO MCP，无新增运行时依赖。

## 9. 风险与控制

| 风险 | 控制措施 |
| --- | --- |
| 新关键词过宽导致误判 | 使用组合条件，并为每条规则增加反例 |
| Primary 接口变更影响旧调用 | 保留旧函数作为兼容包装，新入口使用上下文接口 |
| 重规划在多个 Primary 间循环 | 保持最多一次重规划，并要求新 Primary 与旧值不同 |
| 黄金集只覆盖固定问法 | 每类增加同义表达和否定反例，不以 20 条作为唯一测试 |
| 实体别名覆盖动作词 | 动作分类优先决定路径，实体仅负责填槽和消歧 |

## 10. 完成定义

当严格评测、全部 Go 测试和 MCP 冒烟测试同时通过，且没有放宽断言或通过显式 Primary 绕过分类器时，本修复视为完成。
