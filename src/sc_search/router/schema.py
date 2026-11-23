"""Route request / plan shapes (no LLM — client fills primary+slots)."""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any

from sc_search.query.intent_tools import INTENT_TOOLS


@dataclass
class RoutePlan:
    primary: str
    slots: dict[str, Any]
    question: str
    calls: list[dict[str, Any]]
    path: str
    notes: str = ""
    replan_count: int = 0
    plan_intent: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


def validate_route(
    primary: str | None,
    slots: dict[str, Any] | None,
) -> tuple[str, dict[str, Any]]:
    """Normalize primary+slots; unknown primary → other."""
    key = (primary or "").strip()
    if key not in INTENT_TOOLS:
        key = "other"
    pb = INTENT_TOOLS[key]
    allowed = set(pb.get("slots") or [])
    raw = slots if isinstance(slots, dict) else {}
    cleaned: dict[str, Any] = {}
    for k, v in raw.items():
        if k not in allowed:
            continue
        if v is None:
            continue
        if isinstance(v, str) and not v.strip():
            continue
        cleaned[k] = v.strip() if isinstance(v, str) else v
    return key, cleaned
