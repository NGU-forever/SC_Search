from __future__ import annotations

import re

_OBTAIN_RE = re.compile(
    r"(哪里买|在哪买|能买到|买得到|多少钱|价格|报价|哪[买卖出]|买点|卖点|"
    r"获取|怎么得|如何得|掉落|奖励|蓝图|制造|制作|配方|材料|"
    r"\bbuy\b|\bsell\b|\bprice\b|\bcrafting\b|\bblueprint\b|买)",
    re.I,
)

_CRAFT_MATERIALS_RE = re.compile(r"(材料|配方|制造|制作|crafting)", re.I)

_GUIDE_INTENTS = frozenset(
    {
        "howto",
        "client_help",
        "earn_meta",
        "pledge_meta",
        "event_guide",
        "trade_route",
        "ship_shop",
    }
)

_OPEN_GUIDE_CONFIRM = "要用浏览器打开推荐教程吗？"

_GUIDE_REASONING = {
    "howto": "操作/找路类：用外链与常识按步骤讲清，勿编造当前补丁独有 UI 细节。",
    "client_help": "客户端/报错：先列常见排查方向（USER/Reset/换区），具体错误码以教程为准。",
    "earn_meta": "赚钱开荒：按玩法类型组织建议；可用任务样例推理，勿编造时薪/最优线。",
    "pledge_meta": "氪金名词：解释 CCU/WB/信用点/退款概念，金额与活动以官网为准。",
    "event_guide": "活动本：说明玩法框架与风险，逐步流程引用外链，勿编造刷新点。",
    "trade_route": "跑商航线：可结合本地单价推理；整条最优线指向规划站。",
    "ship_shop": "船店/租借：本地给静态参数；购点/租金指向 Everark 等外链。",
}

_PLACEHOLDER_VERSIONS = frozenset(
    {"scw-live", "wiki-live", "aliases", "localization-full", "unknown"}
)

_CARGO_KEYS = ("cargo_capacity", "cargo", "scu", "货舱")

_FACT_KEY_ZH = {
    "cargo_capacity": "货舱",
    "cargo_scu": "货舱",
    "cargo": "货舱",
    "manufacturer": "制造商",
    "manufacturer_code": "制造商代码",
    "length": "长度",
    "beam": "宽度",
    "height": "高度",
    "mass": "质量",
    "mass_total": "质量",
    "size": "尺寸",
    "type": "类型",
    "career": "职业",
    "foci": "定位",
    "production_status": "生产状态",
    "crew": "乘员",
    "hp": "结构值",
}

_SOFT_VERIFY = "要用浏览器核实吗？"

_ZH_KEY_HINTS = (
    ("厂家", ("manufacturer",)),
    ("制造商", ("manufacturer",)),
    ("长度", ("length", "beam", "height")),
    ("多长", ("length",)),
    ("宽", ("beam",)),
    ("高", ("height",)),
)

_RELATION_LIMIT = 5

_BRIEF_FACT_LIMIT = 5

_FULL_FACT_LIMIT = 25

_BRIEF_PRICE_LIMIT = 3

_FULL_PRICE_LIMIT = 10

_BRIEF_DOC_LIMIT = 2

_FULL_DOC_LIMIT = 5

_DEEP_ORGANIZE_HINT = (
    "有本地证据时：结合 answer_draft 与 evidence 做深度整理，"
    "写成更详细的玩家可读说明（结论、分点/小表、版本来源、操作含义）；"
    "数值与店名等只能来自证据，勿编造未收录字段。"
)

_HARD_FACT_REASONING = _DEEP_ORGANIZE_HINT

_BLUEPRINT_REASONING = (
    "蓝图获取/制造：按 evidence.acquisition 组织成文；"
    "shop_buyable=unknown 时说明无商店报价、勿断言绝对买不到；"
    "有 missions/materials 则引用；刷点/掉率/矿点库无则明说，勿编造。"
)
