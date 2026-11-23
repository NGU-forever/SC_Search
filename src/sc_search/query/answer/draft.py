from __future__ import annotations

from sc_search.query.answer.constants import (
    _BLUEPRINT_REASONING,
    _DEEP_ORGANIZE_HINT,
    _GUIDE_REASONING,
    _PLACEHOLDER_VERSIONS,
)

def _draft_live_suffix(live: str, source_ver: str | None = None) -> str:
    """Show Live as authority; mention stale source tags (e.g. UEX 3.24.2) separately."""
    if not live:
        return ""
    src = (source_ver or "").strip()
    if (
        src
        and src not in _PLACEHOLDER_VERSIONS
        and src != live
        and not src.startswith(f"{live}-")
        and not src.startswith(f"{live}.")
    ):
        return f"（Live {live}；来源标注 {src}）"
    return f"（Live {live}）"

def _draft_shop_unknown(label: str, live: str, src_ver: str | None = None) -> str:
    return (
        f"{label} 本地知识库未收录游戏内商店/aUEC 报价，"
        f"无法确认哪里买或多少钱{_draft_live_suffix(live, src_ver)}。"
    )

def _draft_ship_shop(label: str, rows: list[dict], live: str) -> str:
    if not rows:
        return _draft_shop_unknown(label, live)
    bits = []
    src_ver = None
    for r in rows[:5]:
        term = r.get("terminal") or "?"
        price = r.get("price")
        try:
            price_s = f"{int(price):,}" if price is not None else "?"
        except (TypeError, ValueError):
            price_s = str(price)
        bits.append(f"{term} 约 {price_s} aUEC")
        if src_ver is None:
            src_ver = r.get("game_version")
    return (
        f"【供 Agent 深度整理】{_DEEP_ORGANIZE_HINT}\n"
        f"{label} 游戏内可购："
        + "；".join(bits)
        + f"{_draft_live_suffix(live, src_ver)}。"
    )

def _format_guide_links(resources: list[dict], *, limit: int = 4) -> str:
    lines: list[str] = []
    for r in resources[:limit]:
        title = r.get("title") or "链接"
        url = r.get("url") or ""
        if url:
            lines.append(f"- {title}：{url}")
    return "\n".join(lines)

def _draft_guide(
    intent: str,
    *,
    label: str,
    live: str,
    resources: list[dict],
    mission_bits: list[str] | None = None,
    static_bit: str | None = None,
) -> str:
    hint = _GUIDE_REASONING.get(intent, "结合本地证据与外链推理作答，勿编造库外数值。")
    chunks: list[str] = [f"【供 Agent 深度整理】{hint}"]
    if label:
        chunks.append(f"相关实体：{label}。")
    if static_bit:
        chunks.append(static_bit)
    if mission_bits:
        chunks.append("本地任务样例：" + "；".join(mission_bits[:4]) + "。")
    if intent == "earn_meta":
        chunks.append(
            "常见开荒方向（需结合版本）：货运/快递熟悉操作 → 低危赏金 → "
            "采矿或大宗货运；收益随补丁波动，以下外链供核对。"
        )
    elif intent == "client_help":
        chunks.append(
            "常见排查方向：删 USER、换服务器区域、启动器重登、官网页角色 Reset；"
            "具体报错以教程为准。"
        )
    elif intent == "ship_shop":
        chunks.append("本地若无 aUEC 店价，请用下方购点表核对；静态船参数可继续追问。")
    link_block = _format_guide_links(resources)
    if link_block:
        chunks.append("参考外链（引用，勿当作库内事实数值）：\n" + link_block)
    if live:
        chunks.append(f"版本参考 Live {live}。")
    return "\n".join(chunks)

def _format_material_line(m: dict) -> str:
    name = str(m.get("name") or "")
    qty = m.get("qty")
    unit = str(m.get("unit") or "")
    if qty is None:
        return name
    if isinstance(qty, float) and qty.is_integer():
        qty = int(qty)
    if unit == "SCU":
        return f"{name} {qty} SCU"
    if unit == "count":
        return f"{name} ×{qty}"
    return f"{name} {qty}{(' ' + unit) if unit else ''}".strip()

def _draft_blueprint_scaffold(
    label: str,
    pool: str | None,
    live: str,
    *,
    missions: list[dict] | None = None,
    materials: list[dict] | None = None,
    wants_materials: bool = False,
    missing: list[str] | None = None,
) -> str:
    """Evidence pack + short hint; Agent does the readable write-up."""
    chunks: list[str] = [f"【供 Agent 深度整理】{_BLUEPRINT_REASONING}"]
    if label:
        chunks.append(f"实体：{label}。")
    chunks.append("商店报价：本地未找到（shop_buyable=unknown）。")
    if pool:
        chunks.append(f"奖励池：{pool}。")
    missions = missions or []
    if missions:
        bits: list[str] = []
        for m in missions[:5]:
            title = m.get("title") or m.get("canonical_name")
            meta = []
            if m.get("contractor"):
                meta.append(str(m["contractor"]))
            if m.get("system"):
                meta.append(str(m["system"]))
            meta_bit = f"（{'，'.join(meta)}）" if meta else ""
            bits.append(f"{title}{meta_bit}")
        chunks.append("任务线索：" + "；".join(bits) + "。")
    materials = materials or []
    if materials:
        joined = "、".join(_format_material_line(x) for x in materials)
        chunks.append(f"制造材料：{joined}。")
        if any(str(x.get("ingredient_kind") or "") == "resource" for x in materials):
            chunks.append("资源类刷点/矿点：库未收录。")
    miss = set(missing or [])
    if wants_materials and "crafting_recipe" in miss:
        chunks.append("库无制造配方/材料来源。")
    if live:
        chunks.append(f"版本参考 Live {live}。")
    return "\n".join(chunks)
