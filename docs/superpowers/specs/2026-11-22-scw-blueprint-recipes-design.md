# SCW Blueprint 配方 ingest + obtain 草稿材料

日期：2026-11-22  
状态：执行中（完成 CQ7「材料怎么得」缺口）  
前置：`2026-11-22-sc-answer-blueprint-obtain-token-design.md`

## 目标

1. 从 `https://api.star-citizen.wiki/api/blueprints` 分页同步配方（列表已含 `ingredients`）。  
2. 挂到已有 SCMDB `blueprint` 实体（按 `scmdb-bp-{uuid}` 或 `output_name`）；否则新建 `scw-bp-{uuid}`。  
3. `sc_answer` obtain 路径：有 `blueprint_requires` 则写出材料清单，**不再** `missing: crafting_recipe`。  
4. 材料「怎么得」：资源类标注为采矿/精炼获取；**不编造**具体矿点（库无矿点则一句说明）。

## 非目标

- 品质滑条 / 部件槽位 modifier 全量建模。  
- 矿点 top-N 表。  
- 改 MCP 工具签名。

## 数据形状

- source: `scw_blueprint`；game_version 固定标签 `scw-blueprint-live`  
- fact: `craft_time_seconds`（num）、`craft_time_label`（text）  
- relation: `blueprint_requires` blueprint → commodity/item，`qty` = SCU 或件数；`meta_json`: `{"unit":"SCU"|"count","ingredient_kind":"resource"|"item"}`

## CLI

`python -m sc_search.ingest sync scw_blueprints`

## 验收

- CQ7 原句 draft 含 Aluminum / Hephaestanite / Iron 与用量；无 `crafting_recipe` missing。  
- 无配方蓝图仍 missing。  
- 既有 JS-400 / 黄金用例不回归。
