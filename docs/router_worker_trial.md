# Router/Worker 实测记录（rw CLI）

相对 commit `0d159a8`；命令：`python -m sc_search.rw …`

## bypass

- 命令: `python -m sc_search.rw bypass 卡拉克货舱多大`
- 退出码: `0`  耗时: 1093 ms
- ok=True
- draft: 【供 Agent 深度整理】有本地证据时：结合 answer_draft 与 evidence 做深度整理，写成更详细的玩家可读说明（结论、分点/小表、版本来源、操作含义）；数值与店名等只能来自证据，勿编造未收录字段。
卡拉克（Anvil Carrack） 的 货舱 为 456 SCU（Live 4.10.3）。需要更

## trade

- 命令: `python -m sc_search.rw run 黄金哪里卖最贵 --primary trade --slots {"commodity":"黄金","side":"sell"} --compact`
- 退出码: `0`  耗时: 539 ms
- primary=`trade` path=`mcp_only`
- DAG: c1:sc_best_prices needs=[]
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=1 aug=[]
- hint: 单货物价优先 sc_answer；指定终端用 sc_terminal_prices；side 默认卖出语境看问句
- results:
  - `c1` sc_best_prices → OK

## ship_stats

- 命令: `python -m sc_search.rw run 卡拉克货舱和厂家 --primary ship_stats --slots {"ship":"卡拉克"} --compact`
- 退出码: `0`  耗时: 453 ms
- primary=`ship_stats` path=`mcp_only`
- DAG: r1:sc_resolve_name needs=[], c2:sc_entity_facts needs=['r1']
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=2 aug=[]
- hint: 静态度量（货舱/厂家等）；命中实体后可只留 entity_facts
- results:
  - `r1` sc_resolve_name → OK
  - `c2` sc_entity_facts → OK

## mission_rep

- 命令: `python -m sc_search.rw run 红风物流有哪些任务 --primary mission_rep --slots {"faction":"红风物流"} --compact`
- 退出码: `0`  耗时: 911 ms
- primary=`mission_rep` path=`mcp_only`
- DAG: r1:sc_resolve_name needs=[], c2:sc_list_missions needs=['r1'], c3:sc_entity_facts needs=['r1']
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=3 aug=[]
- hint: 先 resolve 势力英文名；list_missions(faction, rank_keyword)；不编造声望点数/时薪
- results:
  - `r1` sc_resolve_name → OK
  - `c3` sc_entity_facts → OK
  - `c2` sc_list_missions → OK

## earn_meta_口语

- 命令: `python -m sc_search.rw run 萌新怎么赚钱 --primary earn_meta --slots {"activity":"开荒"} --compact`
- 退出码: `0`  耗时: 936 ms
- primary=`earn_meta` path=`mcp_then_browser`
- DAG: c2:sc_search_docs needs=[]
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=1 aug=['sc_answer']
- hint: 赚钱开荒；可列任务线索，禁止编造时薪；外链供核对
- results:
  - `c2` sc_search_docs → FAIL err=no documents matched query='萌新怎么赚钱'
  - `aug_empty_sc_answer` sc_answer [aug] → OK

## earn_meta_Haul

- 命令: `python -m sc_search.rw run 有哪些货运任务可以赚钱 --primary earn_meta --slots {"activity":"Haul"} --compact`
- 退出码: `0`  耗时: 1370 ms
- primary=`earn_meta` path=`mcp_then_browser`
- DAG: c1:sc_list_missions needs=[], c2:sc_search_docs needs=[]
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=2 aug=[]
- hint: 赚钱开荒；可列任务线索，禁止编造时薪；外链供核对
- results:
  - `c2` sc_search_docs → OK
  - `c1` sc_list_missions → OK

## lore_空

- 命令: `python -m sc_search.rw run zzzznotanentity999 --primary lore_docs --slots {"query":"zzzznotanentity999"} --compact`
- 退出码: `2`  耗时: 830 ms
- primary=`lore_docs` path=`mcp_only`
- DAG: c1:sc_search_docs needs=[]
- signals: empty=True slot_retry=True mismatch=False replan=False evidence_count=0 aug=[]
- hint: 证据为空，请改 slots/检索词（英文或标准译名）后重跑；勿把开荒/赚钱等口语当 rank_keyword
- results:
  - `c1` sc_search_docs → FAIL err=no documents matched query='zzzznotanentity999'

## howto

- 命令: `python -m sc_search.rw run 怎么叫船 --primary howto --slots {"topic":"叫船"} --compact`
- 退出码: `0`  耗时: 1209 ms
- primary=`howto` path=`mcp_then_browser`
- DAG: c1:sc_search_docs needs=[]
- signals: empty=False slot_retry=False mismatch=False replan=False evidence_count=1 aug=['sc_answer']
- hint: 操作/新手流程；guides howto + 可有的本地事实
- results:
  - `c1` sc_search_docs → FAIL err=no documents matched query='叫船'
  - `aug_empty_sc_answer` sc_answer [aug] → OK

## 效果对照

| 用例 | 判定 | exit | ms |
| :--- | :--- | ---: | ---: |
| bypass | OK | 0 | 1093 |
| trade | OK | 0 | 539 |
| ship_stats | OK | 0 | 453 |
| mission_rep | OK | 0 | 911 |
| earn_meta_口语 | OK | 0 | 936 |
| earn_meta_Haul | OK | 0 | 1370 |
| lore_空 | SLOT_RETRY | 2 | 830 |
| howto | OK | 0 | 1209 |

## 结论

1. 旁路 / 贸易 / 飞船 / 声望任务：主路径稳定。上表耗时含每次 `python -m` 冷启动（约 0.4–1.4s）；进程内 Worker 本身仍是毫秒级。
2. earn_meta「开荒」：DAG 只有 docs（无无过滤 missions）；docs 空 → 加调 `sc_answer` 成功，故 exit=0、非 slot_retry。
3. 真·空证据（lore 乱码）：exit=2 + `slot_retry_suggested`，软信号正确。
4. howto「叫船」：docs 空后同样靠 `sc_answer` 加调撑住；成文质量仍取决于 answer 草稿，客户端需二次整理。
5. mission_rep 结果顺序 `facts` 先于 `missions`，说明 resolve 后两路并行仍在生效。
6. `--compact` 适合扫信号；要看价目/任务正文用默认 `run`（无 `--compact`）。
