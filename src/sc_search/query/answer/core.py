from __future__ import annotations

import sqlite3

from sc_search.query.entities import entity_facts
from sc_search.query.freshness import live_game_version
from sc_search.query.missions import list_missions
from sc_search.query.plan import query_plan
from sc_search.query.prices import best_prices, terminal_prices
from sc_search.query.search import search_docs

from sc_search.query.answer.acquisition import (
    _acquisition_shop_yes,
    _acquisition_unknown,
    _missions_for_pool,
    _pool_name_from_facts,
    _recipe_materials,
    _ship_buy_prices,
    _wants_obtain_or_price,
)
from sc_search.query.answer.confidence import _needs_soft_verify, _recompute_confidence
from sc_search.query.answer.constants import (
    _BLUEPRINT_REASONING,
    _BRIEF_DOC_LIMIT,
    _BRIEF_FACT_LIMIT,
    _BRIEF_PRICE_LIMIT,
    _CRAFT_MATERIALS_RE,
    _DEEP_ORGANIZE_HINT,
    _FULL_DOC_LIMIT,
    _FULL_FACT_LIMIT,
    _FULL_PRICE_LIMIT,
    _GUIDE_INTENTS,
    _GUIDE_REASONING,
    _HARD_FACT_REASONING,
    _OPEN_GUIDE_CONFIRM,
    _RELATION_LIMIT,
    _SOFT_VERIFY,
)
from sc_search.query.answer.draft import (
    _draft_blueprint_scaffold,
    _draft_guide,
    _draft_live_suffix,
    _draft_ship_shop,
    _draft_shop_unknown,
    _format_guide_links,
)
from sc_search.query.answer.evidence import (
    _display_name,
    _empty_evidence,
    _fact_value,
    _label_fact_key,
    _pick_facts,
    _slim_fact,
    _slim_prices,
    _slim_relations,
    _terminal_from_context,
)

def answer_question(
    conn: sqlite3.Connection,
    question: str,
    detail_level: str = "brief",
    hint_entity: str | None = None,
) -> dict:
    level = (detail_level or "brief").strip().lower()
    if level not in {"brief", "full"}:
        level = "brief"

    plan = query_plan(conn, question, hint_entity=hint_entity)
    intent = plan["intent"]
    evidence = _empty_evidence()
    evidence["entities"] = list(plan.get("entities") or [])
    follow_ups: list[str] = []
    detail_available = False
    draft = ""
    missing: list[str] = []
    had_scored_hit = False

    fact_limit = _BRIEF_FACT_LIMIT if level == "brief" else _FULL_FACT_LIMIT
    price_limit = _BRIEF_PRICE_LIMIT if level == "brief" else _FULL_PRICE_LIMIT
    doc_limit = _BRIEF_DOC_LIMIT if level == "brief" else _FULL_DOC_LIMIT

    primary = evidence["entities"][0]["canonical_name"] if evidence["entities"] else ""
    label = _display_name(evidence["entities"])
    live = live_game_version(conn)
    primary_kind = (
        evidence["entities"][0].get("kind") if evidence["entities"] else None
    )

    if _wants_obtain_or_price(question) and primary_kind == "blueprint" and primary:
        bundle = entity_facts(conn, primary) or {}
        all_facts = bundle.get("facts") or []
        pool = _pool_name_from_facts(all_facts)
        missions = _missions_for_pool(conn, pool)
        entity_row = bundle.get("entity") or {}
        bp_id = entity_row.get("id")
        materials = (
            _recipe_materials(conn, int(bp_id)) if bp_id is not None else []
        )
        wants_materials = bool(_CRAFT_MATERIALS_RE.search(question or ""))
        acq_missing: list[str] = []
        if wants_materials and not materials:
            acq_missing.append("crafting_recipe")
        evidence["acquisition"] = _acquisition_unknown(
            pool,
            missions=missions,
            materials=materials,
            missing=acq_missing,
        )
        evidence["facts"] = [_slim_fact(f) for f in all_facts[:fact_limit]]
        evidence["missing"] = list(acq_missing)
        rels: list[dict] = []
        if missions:
            rels.extend(
                {
                    "rel_type": "mission_reward_blueprint_pool",
                    "from_name": m.get("canonical_name"),
                    "to_name": pool,
                    "qty": None,
                }
                for m in missions[:_RELATION_LIMIT]
            )
        for mat in materials[:_RELATION_LIMIT]:
            rels.append(
                {
                    "rel_type": "blueprint_requires",
                    "from_name": primary,
                    "to_name": mat.get("name"),
                    "qty": mat.get("qty"),
                }
            )
        evidence["relations"] = rels
        evidence["reasoning_hint"] = _BLUEPRINT_REASONING
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": "high",
            "live_game_version": live,
            "answer_draft": _draft_blueprint_scaffold(
                label or primary,
                pool,
                live,
                missions=missions,
                materials=materials,
                wants_materials=wants_materials,
                missing=acq_missing,
            ),
            "evidence": evidence,
            "detail_available": False,
            "follow_ups": [],
            "browser": {"needed": False},
            "plan": plan,
        }

    if (
        _wants_obtain_or_price(question)
        and primary_kind in {"ship", "item"}
        and primary
        and intent == "facts"
    ):
        bundle = entity_facts(conn, primary) or {}
        entity_row = bundle.get("entity") or {}
        src_ver = entity_row.get("game_version")
        buy_rows: list[dict] = []
        if primary_kind == "ship":
            seen_terms: set[str] = set()
            for ent in evidence["entities"]:
                if ent.get("kind") != "ship" or ent.get("id") is None:
                    continue
                for row in _ship_buy_prices(conn, int(ent["id"]), limit=price_limit):
                    term = str(row.get("terminal") or "")
                    if term in seen_terms:
                        continue
                    seen_terms.add(term)
                    buy_rows.append(row)
                if len(buy_rows) >= price_limit:
                    break
            buy_rows = sorted(
                buy_rows, key=lambda r: (float(r.get("price") or 0), str(r.get("terminal") or ""))
            )[:price_limit]
        evidence["facts"] = [
            _slim_fact(f) for f in (bundle.get("facts") or [])[:fact_limit]
        ]
        if buy_rows:
            evidence["prices"] = _slim_prices(buy_rows)
            evidence["acquisition"] = _acquisition_shop_yes(shops=_slim_prices(buy_rows))
            evidence["missing"] = []
            evidence["reasoning_hint"] = _HARD_FACT_REASONING
            return {
                "intent": intent,
                "path": plan["path"],
                "confidence": "high",
                "live_game_version": live,
                "answer_draft": _draft_ship_shop(label or primary, buy_rows, live),
                "evidence": evidence,
                "detail_available": bool(evidence["facts"]),
                "follow_ups": [],
                "browser": {"needed": False},
                "plan": plan,
            }
        evidence["acquisition"] = _acquisition_unknown(
            None,
            mode="shop_unknown",
            hint_zh="本地未收录游戏内商店/aUEC 报价",
            missing=["shop_prices"],
        )
        evidence["missing"] = ["shop_prices"]
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": "high",
            "live_game_version": live,
            "answer_draft": _draft_shop_unknown(label or primary, live, src_ver),
            "evidence": evidence,
            "detail_available": bool(evidence["facts"]),
            "follow_ups": [],
            "browser": {"needed": False},
            "plan": plan,
        }

    if intent == "mission_list":
        faction = next(
            (e for e in evidence["entities"] if e.get("kind") == "faction"),
            None,
        )
        fname = str((faction or {}).get("canonical_name") or primary or "")
        rows = list_missions(conn, faction=fname or None, limit=8) if fname else []
        evidence["missions"] = [
            {
                "canonical_name": r.get("canonical_name"),
                "title": r.get("title"),
                "reward_uec": r.get("reward_uec"),
                "systems": r.get("systems"),
                "mission_type": r.get("mission_type"),
            }
            for r in rows
        ]
        if rows:
            bits = []
            for r in rows[:5]:
                title = r.get("title") or r.get("canonical_name")
                reward = r.get("reward_uec")
                sys = r.get("systems")
                extra = []
                if reward is not None:
                    extra.append(f"{int(reward)} aUEC")
                if sys:
                    extra.append(str(sys))
                extra_bit = f"（{'，'.join(extra)}）" if extra else ""
                bits.append(f"{title}{extra_bit}")
            draft = (
                f"{label or fname} 相关任务示例：{'；'.join(bits)}"
                f"{_draft_live_suffix(live)}。"
            )
            detail_available = len(rows) > 5
            if detail_available:
                follow_ups = ["看更多任务"]
        else:
            missing.append("missions")
            draft = f"本地未找到 {label or fname} 的任务列表。"
            detail_available = False
        evidence["missing"] = missing
        confidence = _recompute_confidence(
            intent,
            plan_confidence=plan["confidence"],
            had_scored_hit=False,
            has_entity=bool(evidence["entities"]),
            facts=[],
            prices=[],
            docs=[],
            missing=missing,
        )
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": confidence,
            "live_game_version": live,
            "answer_draft": draft,
            "evidence": evidence,
            "detail_available": detail_available,
            "follow_ups": follow_ups,
            "browser": {"needed": False},
            "plan": plan,
        }

    if intent in _GUIDE_INTENTS:
        browser = dict(plan.get("browser") or {})
        resources = list(browser.get("resources") or [])
        evidence["guides"] = resources
        evidence["reasoning_hint"] = _GUIDE_REASONING.get(intent, "")
        mission_bits: list[str] = []
        static_bit = None
        if intent == "earn_meta":
            rows = list_missions(conn, limit=5)
            evidence["missions"] = [
                {
                    "canonical_name": r.get("canonical_name"),
                    "title": r.get("title"),
                    "reward_uec": r.get("reward_uec"),
                    "systems": r.get("systems"),
                    "mission_type": r.get("mission_type"),
                    "contractor": r.get("contractor"),
                }
                for r in rows
            ]
            for r in rows[:4]:
                title = r.get("title") or r.get("canonical_name")
                mtype = r.get("mission_type")
                reward = r.get("reward_uec")
                bit = str(title)
                if mtype:
                    bit += f"[{mtype}]"
                if reward is not None:
                    bit += f"~{int(reward)}aUEC"
                mission_bits.append(bit)
        if intent == "ship_shop" and primary:
            bundle = entity_facts(conn, primary) or {}
            facts = bundle.get("facts") or []
            cargo = next((f for f in facts if f.get("key") in ("cargo_scu", "cargo_capacity")), None)
            if cargo:
                static_bit = f"静态参考：货舱 {_fact_value(_slim_fact(cargo))}。"
                evidence["facts"] = [_slim_fact(cargo)]
        if not resources:
            missing.append("guides")
        draft = _draft_guide(
            intent,
            label=label or primary,
            live=live,
            resources=resources,
            mission_bits=mission_bits or None,
            static_bit=static_bit,
        )
        follow_ups = [_OPEN_GUIDE_CONFIRM]
        evidence["missing"] = missing
        return {
            "intent": intent,
            "path": plan["path"],
            "confidence": "medium",
            "live_game_version": live,
            "answer_draft": draft,
            "evidence": evidence,
            "detail_available": bool(resources or mission_bits),
            "follow_ups": follow_ups,
            "browser": browser,
            "plan": plan,
        }

    if intent in {"facts", "loadout_calc", "mining_calc"} and primary:
        bundle = entity_facts(conn, primary)
        if bundle:
            all_facts = bundle.get("facts") or []
            all_relations = bundle.get("relations") or []
            picked, had_scored_hit = _pick_facts(all_facts, question, fact_limit)
            evidence["facts"] = picked
            if level == "full" and all_relations:
                slim = _slim_relations(all_relations)
                evidence["relations"] = slim
            detail_available = len(all_facts) > len(picked) or len(all_relations) > len(
                evidence.get("relations") or []
            )
            src_ver = (bundle.get("entity") or {}).get("game_version")
            if intent == "facts":
                if had_scored_hit and picked:
                    evidence["reasoning_hint"] = _HARD_FACT_REASONING
                    top = picked[0]
                    key_label = top.get("key_zh") or _label_fact_key(
                        str(top.get("key") or "")
                    )
                    core = (
                        f"{label} 的 {key_label} 为 {_fact_value(top)}"
                        f"{_draft_live_suffix(live, src_ver)}。"
                    )
                    if detail_available:
                        core += "需要更细的硬点/部件可以说一声。"
                        follow_ups = ["展开全部参数", "只要货舱"]
                    draft = f"【供 Agent 深度整理】{_DEEP_ORGANIZE_HINT}\n{core}"
                else:
                    draft = f"本地库有 {label}，但未找到匹配字段。"
                    missing.append("matching_facts")
            else:
                site = "Erkul" if intent == "loadout_calc" else "采矿工具站"
                static = ""
                if picked:
                    static = f"静态参考：{picked[0].get('key')}={_fact_value(picked[0])}。"
                guide_links = _format_guide_links(
                    list((plan.get("browser") or {}).get("resources") or []),
                    limit=3,
                )
                draft = (
                    f"{label} 配装/计算需 {site}。"
                    f"{static}"
                    "本地只能给静态参数；Agent 可先据静态参数推理建议，"
                    "要精确 DPS/破矿模拟再说，确认后打开工具站；"
                    "打开并摘要后请把浏览器留在会话里（leave_open），方便用户自己继续改。"
                )
                if guide_links:
                    draft += "\n参考：\n" + guide_links
                follow_ups = [f"打开 {site} 查配装", "只要静态参数", _OPEN_GUIDE_CONFIRM]
                detail_available = True
        else:
            missing.append("entity_facts")
            draft = f"未找到 {label or primary} 的本地事实。"

    elif intent == "prices":
        side = "sell"
        for step in plan.get("mcp_steps") or []:
            if step.get("tool") == "sc_best_prices":
                side = (step.get("args") or {}).get("side") or "sell"
        entities = evidence["entities"]
        terminal_name = _terminal_from_context(entities, question)
        commodity_entity = next((e for e in entities if e.get("kind") == "commodity"), None)
        commodity = (commodity_entity or {}).get("canonical_name") or primary or question

        if terminal_name:
            all_rows = terminal_prices(conn, terminal_name, side=side)
            if commodity_entity:
                cn = str(commodity_entity["canonical_name"])
                all_rows = [
                    r
                    for r in all_rows
                    if str(r.get("commodity") or "").lower() == cn.lower()
                ]
            prices_remain = len(all_rows) > price_limit
            rows = all_rows[:price_limit]
        else:
            fetched = best_prices(conn, commodity, side=side, limit=price_limit + 1)
            prices_remain = len(fetched) > price_limit
            rows = fetched[:price_limit]
        evidence["prices"] = _slim_prices(rows)
        if rows:
            evidence["reasoning_hint"] = _HARD_FACT_REASONING
            top = rows[0]
            ver_suffix = _draft_live_suffix(live, top.get("game_version"))
            if terminal_name and commodity_entity is None:
                core = (
                    f"{terminal_name} {side} 报价示例：{top.get('commodity')} "
                    f"约 {top.get('price')} aUEC{ver_suffix}。"
                )
            else:
                core = (
                    f"{top.get('commodity')} {side} 较优：{top.get('terminal')} "
                    f"约 {top.get('price')} aUEC{ver_suffix}。"
                )
            draft = f"【供 Agent 深度整理】{_DEEP_ORGANIZE_HINT}\n{core}"
            detail_available = prices_remain
            if prices_remain:
                follow_ups = (
                    ["看更多终端", "查买入价"]
                    if side == "sell"
                    else ["看更多终端", "查卖出价"]
                )
        else:
            missing.append("prices")
            target = terminal_name or commodity
            draft = f"本地没有 {target} 的 {side} 报价。"
            detail_available = False

    elif intent == "docs":
        rows = search_docs(conn, question, limit=doc_limit)
        evidence["docs"] = [
            {
                "title": r.get("title"),
                "snippet": (r.get("snippet") or (r.get("body") or ""))[:240],
            }
            for r in rows
        ]
        if rows:
            snip = evidence["docs"][0].get("snippet") or ""
            draft = f"资料摘要：{snip[:120]}…" if snip else "找到相关文档。"
            detail_available = True
            follow_ups = ["展开更多 Wiki 摘要"]
        else:
            missing.append("docs")
            draft = "本地 Wiki 摘要未命中。"
            detail_available = False

    elif intent == "live_web":
        draft = "这类信息需要官网或 Spectrum；本地库不覆盖实时 Patch。需要的话再开浏览器。"
        detail_available = False
        follow_ups = ["打开 RSI 查 Patch Notes"]

    else:
        if not primary:
            draft = "本地知识库未覆盖该问题对应实体；请换标准译名或先 sync。"
            missing.append("entity")
            detail_available = False
        elif not draft:
            draft = "暂无法根据本地数据作答。"
            detail_available = False

    evidence["missing"] = missing
    if not draft:
        draft = "本地知识库未覆盖该问题；不会编造数据。"

    has_entity = bool(evidence["entities"])
    confidence = _recompute_confidence(
        intent,
        plan_confidence=plan["confidence"],
        had_scored_hit=had_scored_hit,
        has_entity=has_entity,
        facts=evidence["facts"],
        prices=evidence["prices"],
        docs=evidence["docs"],
        missing=missing,
    )
    browser = dict(plan.get("browser") or {"needed": False})
    soft = _needs_soft_verify(
        intent,
        confidence,
        had_scored_hit=had_scored_hit,
        facts=evidence["facts"],
        prices=evidence["prices"],
        docs=evidence["docs"],
    )
    if soft:
        if _SOFT_VERIFY not in follow_ups:
            follow_ups.append(_SOFT_VERIFY)
        if intent in {"facts", "prices", "docs"}:
            browser = {**browser, "needed": False}

    return {
        "intent": intent,
        "path": plan["path"],
        "confidence": confidence,
        "live_game_version": live,
        "answer_draft": draft,
        "evidence": evidence,
        "detail_available": detail_available,
        "follow_ups": follow_ups,
        "browser": browser,
        "plan": plan,
    }
