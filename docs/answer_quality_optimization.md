# `sc_ask` 回答质量优化设计

状态：进行中（Mission Projector 已落地；收尾：手续费区间标题、占位符清洗、facts 上限 12）  
日期：2026-11-26  
优先级：P0（第一阶段）  
范围：Go MCP 查询结果的证据投影、结构化成文与回答质量评测  
前置设计：[state_machine_orchestration_design.md](state_machine_orchestration_design.md)  
后续阶段：[classification_maintainability_optimization.md](classification_maintainability_optimization.md)、[knowledge_coverage_optimization.md](knowledge_coverage_optimization.md)

## 1. 背景

`sc_ask` 状态机已经能够正确完成分类、Router/Worker 执行、受控重试和浏览器门禁。当前主要问题不再是“有没有查到证据”，而是“证据是否进入用户最终看到的回答”。

典型案例是：

```text
Bit Zeros ship heist 手续费和冷却
```

Worker 返回的 `sc_list_missions` 证据已包含：

- `title`
- `cost_uec`
- `cooldown`
- `lifetime`
- `reward_uec`
- `reputation_min`
- `reputation_rank`
- `systems`
- `pyro_regions`
- `site_pool`

但 `orchestrate.buildBlocks` 当前只专门投影：

- `sc_resolve_name`
- `sc_entity_facts`
- `sc_best_prices`

因此任务证据虽然存在，`answer_blocks` 和 `answer_draft` 仍可能只显示“实体：Bit Zeros”，造成“路由正确、证据正确、回答不完整”。

## 2. 问题定义

### 2.1 Presenter 不理解意图语义

当前 Presenter 主要按工具类型抽取通用键值，不知道不同 Primary 的核心字段：

| Primary | 用户真正关心的字段 |
| --- | --- |
| `mission_money` | 手续费、冷却、奖励、时效、地区、声望门槛 |
| `mission_rep` | 势力、等级、声望门槛、可接任务 |
| `trade` | 买卖方向、终端、价格、版本和陈旧状态 |
| `blueprint` / `crafting` | 获取方式、材料、数量、来源 |
| `ship_stats` | 用户问到的参数，而非任意前 8 个事实 |

统一按“证据出现顺序”截断前 8 个事实，会让低价值字段挤掉用户明确询问的字段。

### 2.2 `answer_blocks` 只有扁平 Facts

`FactBlock{Label, Value}` 适合单实体参数，但不擅长表达：

- 多个任务的对比；
- 同一任务的手续费、冷却、地区等一组字段；
- 多终端价格；
- 蓝图材料列表。

短期可以继续输出扁平 Facts 以保证兼容，长期需要可选的结构化分组。

### 2.3 黄金评测偏重路由，不足以衡量答案

现有黄金集主要断言：

- `expected_primary`
- `must_contain`
- `must_not_contain`
- 浏览器门禁

部分复杂题只验证 Primary，没有验证核心业务字段是否出现在最终文本，因此无法阻止“路径正确但没回答问题”的回归。

## 3. 目标与非目标

### 3.1 目标

1. 最终回答优先呈现用户问到的字段。
2. 所有数字、任务名、店名和版本都必须来自 Evidence。
3. 复杂任务题在 `answer_draft` 中直接给出手续费、冷却等结论。
4. 保持 `answer_blocks.facts`、`answer_draft` 和现有 MCP 契约兼容。
5. stale、missing、partial 和 no-match 语义继续显式呈现。
6. 增加答案级严格评测，避免只检查路由。

### 3.2 非目标

- 不修改 QueryPlan 分类规则。
- 不重写 Router/Worker。
- 不引入 LLM 成文或外部运行时。
- 不在本阶段扩充数据源。
- 不把缺失字段推断成确定事实。

## 4. 方案选择

### 4.1 方案 A：继续增加通用工具 switch

在 `buildBlocks` 中加入 `case "sc_list_missions"`，直接追加任务字段。

优点：改动最小。  
缺点：Presenter 会继续膨胀，无法按 Primary 区分字段优先级。

### 4.2 方案 B：按 Primary 建立确定性证据投影器（推荐）

新增内部投影层：

```text
State + Evidence
  → Project(primary)
  → AnswerBlocks
  → DraftRenderer(primary)
  → answer_draft
```

每个投影器只负责一个回答领域，并复用通用的来源、版本、warning 和 missing 处理。

优点：

- 用户意图与字段选择一致；
- 可独立单测；
- 后续增加 `blueprint`、`trade` 投影时不污染主函数；
- 不需要改变 Worker 和数据库。

缺点：增加少量内部接口和文件。

### 4.3 方案 C：让宿主 Agent直接整理原始 Evidence

优点：服务端代码少。  
缺点：不同宿主行为不一致，重新引入状态机 v1 已经消除的问题。

结论：采用方案 B。

## 5. 建议架构

### 5.1 文件边界

```text
go/internal/orchestrate/
├── present.go                 # 总流程、通用 warning/missing/version
├── present_project.go         # Projector 接口和通用 Evidence 访问器
├── present_missions.go        # mission_money / mission_rep
├── present_trade.go           # trade / trade_route
├── present_entities.go        # ship_stats / blueprint / crafting
└── present_test.go            # Presenter 单元测试
```

暂不要求一次拆出所有文件。第一批只实现 `present_project.go` 和 `present_missions.go`，其余逻辑保持原位，等待实际修改时再迁移。

### 5.2 内部接口

建议使用小型内部接口，不暴露到 MCP：

```go
type Projection struct {
    Headline string
    Facts    []FactBlock
    Draft    string
    Missing  []string
    Warnings []string
}

type EvidenceProjector interface {
    Supports(primary string) bool
    Project(st *State) Projection
}
```

约束：

- Projector 只能读取 `State.Answer`、`State.Evidence`、`State.Meta` 和用户问题。
- Projector 不访问数据库，不执行工具，不改变路由。
- Projector 不生成 Evidence 中不存在的数值。
- 通用 stale、partial、failed_calls 处理仍由 `present.go` 完成。

### 5.3 Mission 投影规则

`mission_money` 按用户问题中的字段词确定优先级：

| 问法包含 | 优先字段 |
| --- | --- |
| 手续费 / buy-in / cost | `cost_uec` |
| 冷却 / cooldown | `cooldown` |
| 奖励 / 收益 | `reward_uec` |
| 时效 / lifetime | `lifetime` |
| 声望 / 等级 | `reputation_min`、`reputation_rank` |
| 站点 / 地区 | `site_pool`、`pyro_regions`、`systems` |

任务行筛选顺序：

1. 任务标题或 canonical name 与用户活动词匹配；
2. 系统、势力和地区槽位匹配；
3. 有用户所问字段的任务优先；
4. 最多展示 3 条，避免把全部任务列表倾倒进回答。

字段单位固定：

- `cost_uec`、`reward_uec`：`aUEC`
- `cooldown`：分钟
- `lifetime`：小时
- `reputation_min`：只标“最低声望值”，不自行解释为游戏内可见进度

当同一势力有多条同类任务时，必须带任务标题或难度/地区，避免把不同任务的手续费和冷却拼成一条。

### 5.4 兼容策略

第一阶段不删除或改名现有 JSON 字段：

- 继续返回 `answer_blocks.headline`
- 继续返回 `answer_blocks.facts`
- 继续返回 `answer_blocks.warnings`
- 继续返回 `answer_draft`

如需增加分组结构，只能新增可选字段，例如：

```go
Sections []AnswerSection `json:"sections,omitempty"`
```

新增字段不是第一批实现的必要条件。先证明扁平 Facts + 专用 Draft 已能解决用户可见问题。

## 6. 实施步骤

### 阶段 1：建立回答级失败基线

1. 扩展黄金题：
   - Bit Zeros ship heist 必须包含 `13500` 或匹配任务的手续费；
   - 必须包含 `45` 和“分钟”；
   - 不得只返回势力实体摘要。
2. 增加 Presenter 单测，直接构造 `sc_list_missions` Evidence。
3. 验证测试在现有实现上失败。

### 阶段 2：实现 Mission Projector

1. 增加 Evidence 工具结果读取助手。
2. 增加任务行匹配和字段优先级。
3. 生成任务专用 Facts、Headline 和 Draft。
4. 保留通用 warning、stale、missing 拼接。

### 阶段 3：扩展到高价值现有领域

按评测结果依次处理：

1. `mission_rep`
2. `trade`
3. `blueprint` / `crafting`
4. `ship_stats` 的问句字段优先

没有失败样本的领域不提前重构。

### 阶段 4：答案质量回归

1. 扩充 `eval/questions.jsonl` 的内容断言。
2. 增加 no-match、partial、stale 和多任务反例。
3. 运行全量测试和 MCP 冒烟。

## 7. 测试策略

### 7.1 单元测试

- Mission Evidence 能投影 `cost_uec` 和 `cooldown`。
- 用户只问手续费时，不要求冷却字段存在。
- 多任务结果不会跨任务混合字段。
- 空字段不输出 `0` 或 `<nil>`。
- stale、failed_calls 和 missing 不会被专用投影器吞掉。

### 7.2 黄金评测

建议给黄金集增加可选字段：

```json
{
  "question": "Bit Zeros ship heist 手续费和冷却",
  "expected_primary": "mission_money",
  "must_contain": ["13500", "45", "分钟"],
  "must_not_contain": ["请改用 sc_route"]
}
```

若数据库版本导致具体任务数值变化，应使用专门的结构断言，而不是放宽成只检查 Primary。评测器可增加：

- `must_have_fact_labels`
- `must_have_source_tools`
- `must_report_units`

### 7.3 回归测试

```bash
cd go
go test ./internal/orchestrate ./internal/query ./internal/worker -count=1
go test ./... -count=1
```

MCP 冒烟至少覆盖：

- 任务手续费和冷却；
- 单商品最优卖价；
- 势力等级任务；
- 蓝图材料；
- 无匹配实体；
- stale 价格。

## 8. 验收标准

1. Bit Zeros 示例直接回答匹配任务的手续费和冷却。
2. 数值均可在返回 Evidence 中定位。
3. 多任务证据不发生字段串行或错配。
4. 现有 20 条黄金题继续严格通过，并增加答案级断言。
5. 简单 `answer_bypass` 行为不退化。
6. stale、missing、partial 和浏览器门禁不退化。
7. 全量 Go 测试通过。

## 9. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 专用模板数量失控 | 只为有明确失败样本的 Primary 增加 Projector |
| 数值跨任务混合 | 以单条 mission row 为最小投影单位 |
| 具体数值随数据同步变化 | 同时验证字段、单位、来源；必要时用测试 fixture |
| API 兼容性破坏 | 第一批不改现有字段，只改变内容完整度 |
| 回答过长 | 默认最多 3 条任务、8 个 Facts，详细内容通过 follow-up 展开 |

## 10. 完成定义

当复杂问题的核心 Evidence 能稳定进入 `answer_blocks` 和 `answer_draft`，且答案级黄金评测能够阻止“路由正确但没有回答问题”的回归时，本阶段完成。
