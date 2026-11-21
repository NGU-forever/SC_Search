# UEX 飞船游戏内购价同步

## 目标

本地库能回答「某某船游戏里哪里买 / 多少 aUEC」，数据来自 UEX `vehicles_purchases_prices_all`（与现有 `UEX_API_TOKEN` 相同）。

## 范围

- **做**：同步车辆列表对齐 `kind=ship`；写入 `prices`（`side=buy`，`source=uex`）；终端复用 UEX terminals；`sc_answer` 对船/获取问法优先用这些价。
- **不做（一期）**：Everark 爬取、官网 Pledge/$（`vehicles_prices`）。

## 数据流

1. `GET vehicles` + `GET vehicles_purchases_prices_all`（+ 已有 terminals）。
2. 将 UEX 船名对齐已有 SCW 船实体（按 canonical / alias / name_full）；无匹配则 `upsert` 新 ship。
3. `replace_price(commodity_id=ship_entity_id, …)`（复用表，实体为船）。
4. `answer_question`：船 + 买/价问法时查该实体 buy 价；有则 `shop_buyable=yes` 并起草店名+价；无则维持 `shop_unknown`。

## 版本

UEX `game_version` 可能滞后；短答继续带 `live_game_version`，并在草稿中标注 UEX 源版本（与商品价一致）。

## CLI

并入现有 `python -m sc_search.ingest sync uex`（`sync_all.sh` 自动带上）。
