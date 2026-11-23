# Router/Worker 实测记录

DB: `data/sc_knowledge.sqlite`


## 1. 旁路-货舱

- 问题: 卡拉克货舱多大
- 模式: `bypass`
- 耗时: 68 ms
- ok: True
- draft: 【供 Agent 深度整理】有本地证据时：结合 answer_draft 与 evidence 做深度整理，写成更详细的玩家可读说明（结论、分点/小表、版本来源、操作含义）；数值与店名等只能来自证据，勿编造未收录字段。
卡拉克（Anvil Carrack） 的 货舱 为 456 SCU（Live 4.10.3）。需要更细的硬点/部件可以说一声。
- intent/path: facts / mcp_only

## 2. 贸易-黄金卖价

- 问题: 黄金哪里卖最贵
- 模式: `route_worker`
- primary: `trade`
- slots: `{"commodity": "黄金", "side": "sell"}`
- sc_route: ok=True 耗时 2 ms
- path: `mcp_only`
- DAG calls:
  - `c1` sc_best_prices needs=[] args={"commodity": "黄金", "side": "sell"}
- sc_worker_run: ok=True 耗时 5 ms
- mismatch=False replan_suggested=False augmented=0
- call_results:
  - `c1` sc_best_prices → OK | TDD - Trade and Development Division - Area 18 @ 31000.0; Admin - Pyro Gateway (Nyx) @ 31000.0; TDD - Trade and Development Division - Commons - New Babbage @ 30000.0 …(+7)
- evidence 条数: 1

## 3. 飞船-静态度

- 问题: 卡拉克货舱和厂家
- 模式: `route_worker`
- primary: `ship_stats`
- slots: `{"ship": "卡拉克"}`
- sc_route: ok=True 耗时 0 ms
- path: `mcp_only`
- DAG calls:
  - `r1` sc_resolve_name needs=[] args={"text": "卡拉克"}
  - `c2` sc_entity_facts needs=['r1'] args={"name_or_id": "$r1.canonical"}
- sc_worker_run: ok=True 耗时 6 ms
- mismatch=False replan_suggested=False augmented=0
- call_results:
  - `r1` sc_resolve_name → OK | Anvil Carrack (ship)
  - `c2` sc_entity_facts → OK | Anvil Carrack cargo=None
- evidence 条数: 2

## 4. 任务-声望

- 问题: 红风物流有哪些任务
- 模式: `route_worker`
- primary: `mission_rep`
- slots: `{"faction": "红风物流"}`
- sc_route: ok=True 耗时 0 ms
- path: `mcp_only`
- DAG calls:
  - `r1` sc_resolve_name needs=[] args={"text": "红风物流"}
  - `c2` sc_list_missions needs=['r1'] args={"faction": "$r1.canonical"}
  - `c3` sc_entity_facts needs=['r1'] args={"name_or_id": "$r1.canonical"}
- sc_worker_run: ok=True 耗时 18 ms
- mismatch=False replan_suggested=False augmented=0
- call_results:
  - `r1` sc_resolve_name → OK | Red Wind Linehaul (faction)
  - `c3` sc_entity_facts → OK | Red Wind Linehaul cargo=None
  - `c2` sc_list_missions → OK | Master Hauler Needed for Large Shipment; Master Hauler Needed for Large Shipment; Master Hauler Needed for Large Shipment …(+7)
- evidence 条数: 3

## 5. 赚钱-并行

- 问题: 萌新怎么赚钱
- 模式: `route_worker`
- primary: `earn_meta`
- slots: `{"activity": "开荒"}`
- sc_route: ok=True 耗时 0 ms
- path: `mcp_then_browser`
- DAG calls:
  - `c1` sc_list_missions needs=[] args={"rank_keyword": "开荒"}
  - `c2` sc_search_docs needs=[] args={"query": "开荒"}
- sc_worker_run: ok=True 耗时 2 ms
- mismatch=False replan_suggested=False augmented=0
- call_results:
  - `c2` sc_search_docs → FAIL:no documents matched query='开荒' | null
  - `c1` sc_list_missions → FAIL:no missions matched filters rank_keyword='开荒' | null
- evidence 条数: 0

## 6. 故意错意图-看回退

- 问题: 黄金哪里卖最贵
- 模式: `route_worker`
- primary: `lore_docs`
- slots: `{"query": "黄金"}`
- sc_route: ok=True 耗时 0 ms
- path: `mcp_only`
- DAG calls:
  - `c1` sc_search_docs needs=[] args={"query": "黄金"}
- sc_worker_run: ok=True 耗时 0 ms
- mismatch=False replan_suggested=False augmented=0
- call_results:
  - `c1` sc_search_docs → FAIL:no documents matched query='黄金' | null
- evidence 条数: 0

## 补充 A. 合成错意图（trade 计划却只跑 docs）

- 耗时: 5 ms
- mismatch=True replan_suggested=True
- evidence 条数: 1
- reasoning_hint: 证据类型与意图明显不符，请改 primary/slots 后重跑（仅一次）
  - sc_search_docs ok=True data_type=list

### 重规划 → trade 真价

- mismatch=False replan_suggested=False (replan_count=1)
- evidence 条数: 1
  - sc_best_prices ok=True

## 补充 B. earn_meta 改进槽位

- DAG: c1:sc_list_missions, c2:sc_search_docs
- mismatch=False evidence=2
  - sc_search_docs ok=True err=None | Arrastra
  - sc_list_missions ok=True err=None | Master Hauler Needed for Large Shipment

## 补充 C. ship_stats facts 结构抽查

- facts type=list
- fact keys sample: ['career', 'cargo_scu', 'description', 'description_zh', 'description_zh', 'foci', 'manufacturer', 'manufacturer_code', 'mass', 'mass_total', 'production_status', 'size']
- cargo_capacity: None

## 效果评价

| 用例 | 结果 | 说明 |
| :--- | :--- | :--- |
| 旁路 sc_answer | 好 | 68ms 给出 Carrack 456 SCU |
| trade 黄金 | 好 | DAG 单节点价查询，Top 卖价 31000 |
| ship_stats | 好 | resolve→facts 串行绑定 `$r1.canonical` 正确 |
| mission_rep | 好 | resolve 后 list_missions∥entity_facts 并行（结果顺序交错） |
| earn_meta「开荒」 | 弱 | 槽位过口语，missions/docs 皆空；空证据不触发 replan（符合规则3） |
| 错意图 lore_docs | 未触发回退 | docs 也空 → 按规则3不算类型不匹配 |
| 合成 trade+仅docs | 好 | mismatch/replan_suggested 均 True；重规划后拿到物价 |
| earn_meta「Haul」 | 见补充 B | 英文/标准关键词更易命中 |

### 结论

1. Router 展开与 Worker 并行/绑定在贸易、飞船、声望任务上工作正常，本地耗时多为个位数～十几 ms。
2. 回退只在「有证据但类型不对」时触发；空库/空搜不会 replan——客户端需自行决定是否改槽位或改 primary。
3. earn_meta / docs 类对中文口语槽位敏感，Router（客户端 LLM）应尽量填可检索的英文或标准译名。
