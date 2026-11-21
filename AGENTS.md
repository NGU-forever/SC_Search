# 项目规则 (Project Rules) - 星际公民 / Star Citizen

本工作区（Workspace）为专注于游戏 **《星际公民》（Star Citizen，简称 SC）** 的专属项目。在处理本工作区内的所有对话、任务、代码、文档及外部信息交互时，必须严格遵守以下规则：

---

## 1. 检索与搜索强制规范 (Search Protocol)

* **必须附带游戏名称**：
  * 在调用任何外部搜索工具（如 `search_web`）或查找资料时，查询语句（query）**必须包含** `星际公民`、`Star Citizen` 或明确的缩写 `SC`。
  * **严禁发起脱离游戏上下文的泛化搜索**。
    * *错误示例*：`"Carrack best loadout"`、`"Bit Zero 33000"`、`"Pyro IV locations"`
    * *正确示例*：`"Star Citizen" "Carrack" loadout`、`"星际公民" "Bit Zero" 33000`、`"Star Citizen" "Pyro" asteroid bases`
* **中英文混合检索**：
  * 《星际公民》的官方资料、Spectrum 论坛、Reddit 以及 Erkul/SCWiki 多为英文，而国内社区（贴吧、B站等）多用中文俗称。搜索时根据需求灵活组合中英文术语以获取最准确和最新的信息。
* **中文数据检索与 Wiki 对照前置规范 (Chinese Terminology Pre-lookup Protocol)**：
  * **先查 Wiki 对照，再搜中文数据**：当需要查找国内中文平台（B站、贴吧、小黑盒等）或中文网站上的数据与攻略时，**严禁使用自行直译或模糊推测的中文词搜索**。
  * **前置标准映射**：必须先从《星际公民》中文 Wiki（如 `citizenwiki.cn`）中对照确认官方/汉化组统一的标准中英文名称（如飞船名、配件名、任务名、势力名、地点名）。
  * 在确认标准中文译名与社区常用俗称后，再携带规范名称深入中文网站检索，确保搜索命中率与数据准确性。
* **优先从已记录的站点检索 (Priority Sources)**：
  * 检索时优先查阅并在可能时使用 [SOURCES.md](file:///Users/xiongtianyu/Desktop/SC_Search/SOURCES.md) 中记录的用户分享站点及核心工具站，确保信息的权威性与契合度。
* **本地知识库 MCP（优先）**：
  * 涉及物价、终端、货物名称解析、数据新鲜度等**事实类**问题，优先调用本项目 MCP **`sc-search`** 工具：`sc_resolve_name`、`sc_best_prices`、`sc_terminal_prices`、`sc_data_freshness`。
  * 库未覆盖或需交互计算时（Erkul 配装、采矿计算站、Spectrum 等）再使用浏览器；不要编造库中不存在的价格。
  * 写入/同步数据库使用 CLI：`python -m sc_search.ingest sync uex`（MCP 只读）。

---

## 2. 用户分享网站自动沉淀机制 (Site Tracking Protocol)

* **自动识别与记录**：
  * 凡是用户在对话中提到或分享的任何网站、论坛、工具、数据库或文章链接，**必须立即同步追加记录**至项目根目录下的 [SOURCES.md](file:///Users/xiongtianyu/Desktop/Star%20Citizen/SOURCES.md) 中。
  * 记录字段包括：网站名称、完整 URL / 域名、功能或主要涵盖内容、记录日期与说明。
* **优先复用原则**：
  * 在后续需要查找相关信息时，主动利用 `search_web` 的 `domain` 过滤推荐或针对该站点检索，优先从用户沉淀的网站源中获取最新内容。

---

## 3. 领域上下文与知识库 (Domain Context)

* **默认游戏背景**：
  * 用户的任何提问（涉及船只、武器、配件、地点、势力、任务、货币、机制等）均默认在《星际公民》宇宙背景下理解，无需向用户反复确认是否指代《星际公民》。
* **版本与时效性意识**：
  * 《星际公民》处于活跃开发阶段，版本（如 Live、PTU、EPTU；Alpha 3.x 到 4.x Pyro 系统等）迭代频繁，数值和机制常有重大重构（例如 Master Modes 大师模式、货运物理化 Cargo Refactor、飞船软死亡 Soft Death、工程系统 Engineering 等）。
  * 回答机制或推荐配置时，应结合最新版本或明确注明对应版本环境。

---

## 4. 专业术语与表达规范 (Terminology)

* 熟练并准确使用游戏通用术语，包括但不限于：
  * **货币/度量**：aUEC、UEC、SCU（标准货物箱单位）、cSCU。
  * **系统与星系**：Stanton（斯坦顿）、Pyro（狂暴/派罗）、Nyx（尼克斯）、Jump Point（跳跃点）。
  * **机制系统**：QT / Quantum Drive（量子跃迁）、Master Modes（NAV / SCM 模式）、Soft Death（软死亡）、Bed Logging（下床保存）。
  * **制造商与飞船**：RSI、Anvil、Aegis、Drake、MISC、Origin、Crusader、Argo、Consolidated Outland、Mirai、Alien（Banu, Xi'an, Tevarin, Vanduul）等。
* 对常见黑话、俗称和地点简称保持敏感（如“大白鲨”、“仙女座”、“切口/黑切/蓝切”、“十字军/奥里森”、“微凸/新巴贝奇”、“大火星/赫斯顿”、“18区/弧星”、“GH/六角湾”等）。

---

## 5. 回答与文档输出准则 (Output Guidelines)

* **结构化与实用性**：针对玩法攻略、跑商路线、采矿精炼、赏金配装、任务指引等内容，优先给出直观、结构清晰的表格、关键数据点或流程步骤。
* **注重实操要点与风险提示**：主动提示防卡 Bug 技巧、PvP 风险区域、空间站保护区规则（Armistice Zone）以及避免资产损失的实战建议。

---

## 6. 浏览器交互与自动化规范 (Browser Automation Protocol - ego lite)

* **优先选用 ego-browser (ego lite)**：
  * 当任务需要深入浏览外部网站、动态交互或提取非静态索引数据时（如打开并操作 SCMDB、Erkul 配装器、UEX 贸易网、RSI 官网/Spectrum 论坛等），**优先选用 `ego-browser` (ego lite)**。
* **支持的核心操作能力**：
  * **深度数据提取**：直接打开动态 JS 渲染的工具站，提取特定蓝图配方、矿物/货物实时牌价或飞船配件参数。
  * **模拟人机交互**：支持模拟点击、筛选条件勾选、表单填充、滚动翻页以及页面截图。
  * **上下文与登录态复用**：ego lite 支持与用户的日常浏览器环境无缝协同，可借助已登录的用户上下文（如 RSI 官方账号、舰队机库状态等）完成专属查询与操作。
* **操作执行准则**：
  * 遇到需要浏览器自动化的场景，优先调用 `ego-browser` 相关指令或技能，确保高效、稳定地在网页端获取一手最新数据。
