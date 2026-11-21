# Phase ZH: 汉化组 global.ini + 民间俗称

**Goal:** Ingest 汉化组 EN/ZH localization pairs into `aliases` (+ ship `description_zh` facts), expand `seed_aliases.json` slang, CLI `sync zh`.

**Sources:** `https://ini.42kit.com/orginal/global.ini` + `https://ini.42kit.com/full/global.ini` (CC BY-NC-SA 4.0, non-commercial personal use).

**Strategy:** Match short EN display strings to existing entities; add ZH aliases. Prefer vehicle_/items_commodities_ keys; second pass any short EN that resolves to an entity.
