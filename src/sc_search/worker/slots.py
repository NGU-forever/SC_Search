"""Slot / query hygiene for Worker DAG args (colloquial meta → do not search as-is)."""

from __future__ import annotations

import re

# Meta talk: not useful as mission rank_keyword or sole docs query.
_META_RE = re.compile(
    r"^(开荒|刷钱|赚钱|怎么赚|如何赚|萌新|新手|怎么玩|如何玩|"
    r"第一桶金|推荐玩法|有什么玩|干什么|"
    r"earn|money|grind|newbie|starter)$",
    re.I,
)


def is_meta_keyword(text: str | None) -> bool:
    if not text or not str(text).strip():
        return False
    s = str(text).strip()
    if _META_RE.match(s):
        return True
    # Short pure-Chinese meta phrases often appear as activity slots.
    if len(s) <= 6 and re.fullmatch(r"[\u4e00-\u9fff]+", s):
        if any(tok in s for tok in ("赚", "开荒", "萌新", "新手", "刷钱")):
            return True
    return False


def docs_query(slot_val: str | None, question: str) -> str:
    """Prefer concrete slot; fall back to full question when slot is meta/empty."""
    q = (question or "").strip()
    s = (slot_val or "").strip() if slot_val else ""
    if not s or is_meta_keyword(s):
        return q or s
    return s


def mission_rank_keyword(*candidates: str | None) -> str | None:
    for c in candidates:
        if not c or not str(c).strip():
            continue
        if is_meta_keyword(str(c)):
            continue
        return str(c).strip()
    return None
