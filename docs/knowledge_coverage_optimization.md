# SC Search 高频知识覆盖优化设计

状态：待实施  
日期：2026-11-26  
优先级：P2（第三阶段）  
范围：本地知识库的数据域、来源治理、同步流程、检索与覆盖评测  
前置阶段：[answer_quality_optimization.md](answer_quality_optimization.md)、[classification_maintainability_optimization.md](classification_maintainability_optimization.md)  
库完整性前置：[knowledge_db_p0_optimization.md](knowledge_db_p0_optimization.md)（索引 / 同步事务 / UEX 快照与 stale）  
需求依据：[community_question_catalog_zh.md](community_question_catalog_zh.md)

## 1. 背景

当前知识库的优势集中在：

- 中文俗称和中英文实体解析；
- 商品价格；
- 飞船静态参数；
- 势力任务和任务机制字段；
- 蓝图、材料与实体关系；
- 数据新鲜度。

社区高频问题中仍有明显缺口：

1. 操作、找路、叫船和按键；
2. 客户端报错、汉化和账号体系；
3. 当前版本赚钱路线；
4. 游戏内船店 aUEC 价格与购点；
5. 矿点、RS 信号、采矿头和精炼方案；
6. 限时活动逐步流程。

这些领域的数据性质不同，不能统一当作静态 facts：

| 数据域 | 时效 | 适合形式 |
| --- | --- | --- |
| 操作教程 | 中 | documents + FTS |
| 客户端报错 | 高 | 官方文档优先的 documents |
| 赚钱路线 | 很高 | 版本化 guide + 本地结构化证据 |
| 船店购点 | 中高 | 结构化 facts/relations |
| 采矿方案 | 高 | 本地基础事实 + 浏览器计算器门禁 |
| 活动流程 | 很高 | 版本化 documents + 官方链接 |

本阶段重点是建立可持续的数据覆盖方式，而不是把网页内容无差别抓入数据库。

## 2. 原则

### 2.1 证据优先级

1. CIG/RSI 官方资料；
2. 可追踪版本和更新时间的结构化数据；
3. 有明确作者、日期和版本的社区指南；
4. 论坛、视频和帖子只作为问题发现与候选来源，不直接当确定事实。

### 2.2 数据分类

每条内容必须属于以下之一：

- **fact**：结构化、可比较、可用于数值回答；
- **relation**：实体之间的稳定关系；
- **document**：教程、步骤、解释和注意事项；
- **external guide**：必须由宿主确认后打开的交互站或动态页面。

不能把教程观点伪装成 facts，也不能把动态计算结果静态写死。

### 2.3 时效和来源

所有新增数据必须带：

- `source`
- `game_version`
- `ingest_run_id` 或明确的手工版本记录
- 能定位来源的 URL（documents）

不知道适用版本时必须显式标记为 unknown 或 legacy，不默认等于当前 Live。

### 2.4 合规和可发布性

新增源在实施前必须记录：

- 使用方式；
- 许可或公开 API 条款；
- 抓取频率；
- 是否允许随 Release 分发；
- Token 是否可选；
- 失败时是否影响核心同步。

用户分享的新站点继续立即追加到 `SOURCES.md`，但“记录来源”不等于“获准批量入库”。

## 3. 目标与非目标

### 3.1 目标

1. 以社区问题频率确定数据优先级。
2. 为新增知识建立来源、版本、同步和评测闭环。
3. 优先复用现有 `documents` + FTS、facts、relations 和 ingest_runs。
4. 扩展后仍保持单 Go 二进制 + SQLite 的发布方式。
5. 动态或交互内容继续通过 BrowserGate，而不是伪造本地结果。
6. 每个新增数据域都有可自动运行的覆盖评测。

### 3.2 非目标

- 不抓取需要登录、绕过验证或违反站点条款的内容。
- 不把 B 站、贴吧和论坛评论直接作为权威事实。
- 不承诺“全网知识”覆盖。
- 不在服务端运行浏览器。
- 不把社区推荐写成唯一正确答案。
- 不为了新数据源引入常驻 Python/Node 运行时。

## 4. 优先级与阶段

## 4.1 第一批：教程与客户端帮助

原因：

- 社区需求最高；
- 现有 `documents` 和 FTS 已能承载；
- `howto`、`client_help`、`bug_tip` Playbook 已存在；
- 不需要先改变数据库 schema。

覆盖主题：

- 叫船、机库、起飞、降落、量子跃迁、对接；
- 出生点、复活点、常见城市找路；
- 常用按键和设置入口；
- 19000 等常见错误的官方排障入口；
- 汉化安装、失效和版本兼容说明；
- PTU/EPTU 进入条件和官方流程。

实施方式：

1. 建立受审查的本地 guide 文档源；
2. 每篇文档带版本、来源、更新时间和适用平台；
3. 通过现有 `documents` 表入库；
4. `sc_search_docs` 先返回本地摘要；
5. 证据不足时返回 external guide，并保持 `wait_confirm=true`。

## 4.2 第二批：游戏内船店和租赁

原因：

- 问法明确，适合结构化数据；
- 可显著改善 `ship_buy`；
- 比赚钱攻略稳定，更适合本地库。

建议字段：

```text
ship
terminal/store
location
system
purchase_price_uec
rental_price_1d
rental_price_3d
rental_price_7d
rental_price_30d
game_version
source
reported_at
```

优先复用 entities/facts/relations：

- 飞船和商店作为 entity；
- `sold_at` / `rented_at` 作为 relation；
- 价格、租期和 reported_at 放 relation `meta_json` 或专用表。

若查询和唯一性需求证明 `meta_json` 不足，再增加 `ship_shop_prices` 表；不提前扩 schema。

## 4.3 第三批：采矿知识

本地库只保存稳定基础知识：

- 矿物名称和别名；
- 采矿船、激光头、模块的静态参数；
- RS 信号解释和来源；
- 精炼方法的定义和注意事项；
- 外部计算器和速查表导航。

动态组合、最优收益和破矿模拟继续交给：

- Regolith
- RedMonster
- SCMINER
- Cryon Rocks

流程保持：

```text
本地基础证据 → 中文摘要 → wait_confirm → 宿主打开交互站
```

不得把某一版本的“最佳采矿头”永久写成无版本事实。

## 4.4 第四批：赚钱与活动攻略

这是时效风险最高的数据域，最后实施。

内容模型应包含：

- 适用 Live 版本；
- 活动或玩法名称；
- 前置条件；
- 风险和装备要求；
- 证据来源；
- 更新时间；
- 是否为官方机制、结构化数据推导或社区建议。

禁止输出未经证据支持的“每小时收益”。如果只有任务奖励和冷却，可展示原始字段，但不能自动推导真实时薪。

## 5. 建议架构

### 5.1 数据管线

```text
SourceRegistry
  → Fetch
  → Normalize
  → Validate
  → VersionTag
  → SQLite
  → Query
  → sc_ask Presenter
```

每个 ingest source 必须提供：

```go
type SourceManifest struct {
    ID              string
    DataDomains     []string
    License         string
    Redistributable bool
    RequiresToken   bool
    FreshnessPolicy string
}
```

Manifest 可以先用 JSON/YAML 配置表达；只有出现多个新源后才值得增加 Go 类型。

### 5.2 建议文件边界

```text
config/
└── source_manifest.json          # 来源治理信息

data/
└── curated_guides/               # 经审查、可分发的本地指南源

go/internal/ingest/
├── guides.go                     # curated guide 入库
├── shipshops.go                  # 船店源（确认来源后）
└── source_manifest.go            # manifest 校验

go/internal/query/
├── search.go                     # 现有 documents FTS
└── shipshops.go                  # 仅在需要专用结构时新增

eval/
├── questions.jsonl               # 路由与回答黄金题
└── coverage_questions.jsonl      # 数据域覆盖题
```

不要求第一批同时创建全部文件。教程阶段只需 curated guide ingest 与 coverage 评测。

### 5.3 Curated Guide 格式

建议每篇指南使用带 front matter 的 Markdown：

```yaml
id: howto-claim-ship
title: 如何在 ASOP 终端叫船
category: howto
game_version: 4.10
updated_at: 2026-11-26
sources:
  - title: RSI Support
    url: https://...
confidence: official
```

正文要求：

- 使用可执行步骤；
- 区分键位默认值和用户自定义；
- 标出版本差异；
- 不复制无分发许可的全文；
- 外部来源用摘要和链接，不冒充原创事实。

### 5.4 覆盖评测

`coverage_questions.jsonl` 建议字段：

```json
{
  "question": "进不去游戏报错 19000 怎么办",
  "domain": "client_help",
  "expected_primary": "client_help",
  "must_have_evidence": true,
  "must_report_source": true,
  "must_not_claim": ["已为你重置账号"],
  "max_staleness_days": 90
}
```

覆盖评测和分类黄金集分开：

- `questions.jsonl` 检查路由、门禁和核心答案；
- `coverage_questions.jsonl` 检查数据是否存在、来源是否合规、版本是否明确。

## 6. 实施步骤

### 阶段 0：来源审计

1. 为候选数据源记录许可、API、更新频率和分发条件。
2. 删除或降级无法验证来源的候选事实。
3. 确定第一批官方/可分发教程清单。

### 阶段 1：Curated Guide MVP

1. 定义 Markdown 元数据 schema。
2. 实现 guide ingest 到 `documents`。
3. 加入 10–20 篇最高频教程和客户端帮助。
4. 为每篇文档增加来源和版本校验。
5. 增加 coverage 评测。

### 阶段 2：船店结构化数据

1. 确认可用来源和许可。
2. 建立最小字段集和唯一键。
3. 实现增量同步和 stale 策略。
4. 接入 `ship_buy` 查询与 Presenter。
5. 增加价格/购点冲突测试。

### 阶段 3：采矿基础知识

1. 入库稳定实体、别名和静态参数。
2. 加入 RS、精炼和模块说明文档。
3. 保留动态计算 BrowserGate。
4. 增加“静态参数不开浏览器、配置计算需确认”的评测。

### 阶段 4：赚钱与活动版本化指南

1. 只接入带版本和日期的内容。
2. 明确“事实字段”与“社区建议”。
3. 设置更短 stale 阈值。
4. 过期内容降级为 warning，不继续输出“当前最优”。

## 7. 测试策略

### 7.1 Ingest

- 重复同步幂等；
- source/game_version/ingest_run_id 完整；
- 无 URL 或无许可状态的文档不能进入 Release 数据集；
- FTS 触发器在 insert/update/delete 后一致；
- 单个可选源失败不破坏其他同步。

### 7.2 Query

- 中英文、俗称和错误码能检索到正确文档；
- 结果按版本、来源和相关度排序；
- legacy 内容不会压过当前版本；
- 无证据时明确降级，不返回编造步骤。

### 7.3 端到端

至少覆盖：

- 叫船和机库；
- 量子跃迁；
- 19000 错误；
- 汉化失效；
- PTU/EPTU；
- 某船游戏内购点；
- 鼹鼠采矿头；
- 当前版本赚钱；
- 限时活动流程。

## 8. 验收标准

1. 第一批高频教程问题可由本地 documents 给出带来源摘要。
2. 客户端帮助不会执行账号操作或编造排障结果。
3. 船店数据能回答价格、商店和地点，并明确版本。
4. 采矿静态知识本地回答，动态计算继续等待用户确认。
5. 赚钱和活动内容过期时明确 warning，不宣称“当前最优”。
6. 所有新增来源均在 `SOURCES.md` 和来源 manifest 中登记。
7. Release 仍为单 Go 二进制 + SQLite，无新增常驻运行时。
8. 全量测试与 coverage 评测通过。

## 9. 成功指标

建议持续记录：

- 高频题本地证据命中率；
- `status=partial` / `no_match` 比例；
- 因 stale 降级的比例；
- 浏览器门禁确认率；
- 每个数据域的有来源回答比例；
- 数据同步失败率。

首轮目标不追求统一百分比，而是先建立可重复基线。每新增一个数据域，应能证明其核心样本的 no-match 明显下降，同时没有降低来源和版本透明度。

## 10. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 社区攻略快速过期 | 强制版本、更新时间和短 stale 策略 |
| 许可不明确 | 只存摘要和链接，未确认前不随 Release 分发 |
| 数据源过多导致同步脆弱 | 源隔离、可选同步、失败不级联 |
| 攻略观点被当作事实 | confidence/source_type 区分官方事实与社区建议 |
| 动态计算被静态化 | 保留 BrowserGate，库只存基础证据 |
| 覆盖扩张拖累分类 | 先完成第二阶段分类边界，再新增 Intent |

## 11. 完成定义

当高频知识域能够以“来源可追踪、版本可判断、同步可重复、查询可评测”的方式逐批进入本地库，并且动态内容仍由浏览器门禁安全承接时，本阶段完成。
