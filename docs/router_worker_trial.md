# Router/Worker 实测记录（soft-signal 后）

相对 commit `73067c9`；后续含「earn_meta 禁止无过滤任务 dump」的未提交改动。

## 效果对照

| 用例 | 期望 | 结果 |
| :--- | :--- | :--- |
| 旁路 sc_answer | 快路径给货舱 | OK（Carrack 456 SCU） |
| trade 黄金 | 卖价证据 | OK（31000） |
| ship_stats | resolve→facts | OK |
| mission_rep | resolve 后并行 missions∥facts | OK |
| earn_meta「开荒」 | 不塞元话语；不 dump 无过滤任务；docs 用整句；必要时加调 answer | 已修：跳过空 filter `list_missions`；docs 失败后加调 `sc_answer` |
| earn_meta「Haul」 | missions+docs | OK |
| 合成 trade+仅 docs | mismatch + replan | OK；重规划后拿物价 |
| 真·空证据（乱码 query） | evidence_empty + slot_retry | OK |

## 信号语义（锁死）

| 字段 | 何时 |
| :--- | :--- |
| `mismatch` / `replan_suggested` | 有证据但类型与 primary 不符 |
| `evidence_empty` / `slot_retry_suggested` | 无证据；先改 slots/检索词 |
| earn_meta 仅 meta activity | DAG **不含**无过滤 `sc_list_missions` |

## 结论

Router/Worker + 软信号主路径可用。剩余体验问题主要在客户端成文质量与槽位填法，不在并行执行器。
