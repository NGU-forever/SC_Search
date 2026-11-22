# SC Search — 星际公民本地问答助手

在 [豆包](https://www.doubao.com/)（豆包工作）或其它支持 MCP 的 AI 客户端里，用**本地数据库**快速查《星际公民》物价、任务/蓝图、飞船参数和中英俗称，不必每次都翻网页。

适合：边玩边问「黄金哪卖贵」「大白鲨载货多少」「这个任务给什么蓝图」这类事实问题。

---

## 你需要准备什么

| 项目 | 说明 |
| :--- | :--- |
| 电脑 | macOS / Windows / Linux 均可 |
| Python | **3.11 或更高** |
| AI 客户端 | 推荐 [豆包](https://www.doubao.com/)（桌面端「豆包工作」，支持自定义 MCP 连接器）；也可用 Cursor 等任意 STDIO MCP 客户端 |
| UEX 账号（可选） | 要查**实时跑商物价**时需要；只查任务/飞船/Wiki 可不配 |

---

## 5 分钟快速开始

下面命令在**项目根目录**执行（本仓库解压/克隆后的文件夹）。

### 1. 安装

```bash
python3 -m venv .venv

# macOS / Linux
source .venv/bin/activate

# Windows（PowerShell）
# .venv\Scripts\Activate.ps1

pip install -e .
```

### 2. 配置（查物价时需要）

1. 打开 [UEX My Apps](https://uexcorp.space/api/apps)，登录并**创建一个 App**，复制 **App Access Token**。
2. 在项目里复制环境文件并填入 Token：

```bash
cp config/.env.example config/.env
```

编辑 `config/.env`：

```text
UEX_API_TOKEN=粘贴你的Token
```

`SC_SEARCH_DB` 可以留空（默认用 `data/sc_knowledge.sqlite`）。

### 3. 初始化并下载数据

**推荐：一键脚本**（首次或补丁后都可用，大约几分钟，视网速而定）：

```bash
# macOS / Linux
chmod +x scripts/sync_all.sh
./scripts/sync_all.sh
```

```powershell
# Windows PowerShell
.\scripts\sync_all.ps1
```

脚本会依次执行：`init` → `uex` → `scmdb` → `scw` → `wiki` → `aliases` → `zh`。  
若未配置 `UEX_API_TOKEN`，物价同步会跳过，其它源照常进行。

也可以手动逐步执行：

```bash
python -m sc_search.ingest init
python -m sc_search.ingest sync uex                 # 跑商物价 + 飞船游戏内购价（需 Token）
python -m sc_search.ingest sync scmdb --channel live  # 任务 / 蓝图奖励
python -m sc_search.ingest sync scw                 # 飞船参数
python -m sc_search.ingest sync wiki                # 英文 Wiki 摘要
python -m sc_search.ingest sync aliases             # 民间俗称（大白鲨等）
python -m sc_search.ingest sync zh                  # 汉化组标准中文译名（global.ini）
python -m sc_search.ingest status
```

### 4. 接到豆包（推荐）

豆包桌面端「豆包工作」支持自定义 **STDIO** MCP；自定义连接器仅在**本地电脑**任务里可用（不要选云电脑）。

1. 先确认本机能启动 MCP（仓库根目录）：

```bash
# macOS / Linux
./scripts/run_mcp.sh
# 或
.venv/bin/python -m sc_search.mcp_server
```

能挂起等待（stdio）说明服务正常，用 `Ctrl+C` 结束后再去豆包里配。

2. 打开豆包 → **技能 · 连接器 · 伙伴**（或「插件 · 技能 · 伙伴」）→ **新建** → **新建自定义连接器**，按下面填写：

| 字段 | 填写 |
| :--- | :--- |
| 服务器名称 | `sc-search` |
| 传输类型 | **STDIO** |
| 命令 | 本机 venv 的 Python **绝对路径**，例如 `/绝对路径/SC_Search/.venv/bin/python`（Windows：`...\SC_Search\.venv\Scripts\python.exe`） |
| 参数 | 逐项添加：`-m`、`sc_search.mcp_server` |
| 环境变量 | `SC_SEARCH_DB` = `/绝对路径/SC_Search/data/sc_knowledge.sqlite`；`PYTHONPATH` = `/绝对路径/SC_Search/src`（可选，若用 `pip install -e .` 且已激活同环境，有时可不设） |

更省事：命令填仓库里的 `scripts/run_mcp.sh`（先 `chmod +x`），参数留空；脚本会自己解析仓库根与数据库路径。Windows 可继续用上面的 `python.exe` + `-m` 方式。

3. 保存后确认连接器已启用；新建**工作任务**，运行位置选 **本地电脑**，在连接器里勾选 `sc-search`。若首次提示运行失败，在连接器列表里点一次「重新启动」。

也可参考官方说明：[豆包工作 · 插件 / 自定义连接器](https://www.doubao.com/work/docs/zh-cn/articles/705018132598-plugins)。

<details>
<summary>备用：Cursor 等 mcp.json 客户端</summary>

```bash
cp .cursor/mcp.json.example .cursor/mcp.json
```

编辑 `.cursor/mcp.json`（已被 git 忽略，勿提交本机路径），把其中的绝对路径改成你的仓库路径后，在客户端重载 MCP。也可把 `command` 换成 `scripts/run_mcp.sh`。

</details>

### 5. 开始提问

回答链路：Agent 理解意图与槽位 → `sc-search` MCP 检索证据 → Agent 依据证据成文（简单题可直接 `sc_answer`）。

在豆包对话里直接问即可（建议显式选中 `sc-search` 连接器）。Agent 应优先调用本项目的工具，例如：

| 你想问的 | 大致会用到的工具 |
| :--- | :--- |
| （任意 SC 事实问题，默认） | `sc_answer`（一键：归类+查库+短答草稿） |
| （只要路由/调试） | `sc_query_plan`（意图路由：MCP vs 浏览器） |
| 「黄金在哪卖贵？」 | `sc_best_prices` |
| 「Lorville 都卖什么价？」 | `sc_terminal_prices` |
| 「大白鲨是哪艘船？」 | `sc_resolve_name` |
| 「Carrack 载货 / 厂家？」 | `sc_entity_facts` |
| 「红风 Junior 能接什么任务？」 | `sc_list_missions` |
| 「Carrack 是什么船？」 | `sc_search_docs` |
| 「数据是什么时候同步的？」 | `sc_data_freshness` |

也可以自己先确认工具是否正常：问一句「用 sc-search 查一下黄金卖出价」。

---

## 游戏更新后怎么办

补丁或物价变化后，再跑一次一键脚本即可：

```bash
./scripts/sync_all.sh          # macOS / Linux
# .\scripts\sync_all.ps1       # Windows
```

也可以只同步某一源，例如 PTU：`python -m sc_search.ingest sync scmdb --channel ptu`。


MCP **不会**自动改库，避免误触发大流量下载；同步请在终端手动执行。

---

## 现在能查什么 / 还不能指望什么

**本地库比较擅长：**

- 货物买卖价、终端报价（来自 UEX，社区众包，可能有误差）
- 飞船游戏内购点与 aUEC（同属 `sync uex`：UEX `vehicles_purchases_prices_all`）
- 任务赏金、蓝图奖励池、部分物品奖励（SCMDB）
- 飞船参数与中英描述片段（Star Citizen Wiki API）
- Wiki 英文摘要全文检索
- 汉化组标准中文译名（如 克拉克 → Carrack；来自 [汉化组 global.ini](https://citizenwiki.cn/Localization)，CC BY-NC-SA 4.0，仅个人非商用）
- 民间俗称（如大白鲨 → Carrack；维护在 `data/seed_aliases.json`）

**仍建议开网页 / 游戏内确认的：**

- Erkul 精细配装与 DPS
- 采矿阻力、激光头计算（Regolith 等）
- Spectrum 论坛、需登录的官方内容
- 中文维基长文（`citizenwiki.cn` 对自动程序常有 Cloudflare 拦截；标准译名已用汉化组 ini 覆盖）


回答里若带了 `game_version` / 同步时间，请以那一行判断数据新不新。UEX 返回的版本号有时会偏旧，以同步时间和实机为准。

---

## 常见问题

**Q：没有 UEX Token 能用吗？**  
能。跳过 `sync uex`，任务、飞船参数、Wiki、俗称照样可用；只是没有跑商物价和飞船游戏内 aUEC 购点。

**Q：Token 安全吗？**  
放在 `config/.env`，已被 git 忽略，不要发到聊天或公开仓库。

**Q：MCP 里看不到 sc-search / 发现失败？**  
1. **豆包**：确认传输类型是 STDIO、命令为 Python/脚本的**绝对路径**、任务环境是**本地电脑**，连接器已启用；失败时点「重新启动」。  
2. **Cursor 等**：确认 `.cursor/mcp.json` 用的是绝对路径的 `.venv/bin/python`（或 `run_mcp.sh`）。  
3. 依赖需为 **MCP Python SDK 1.x**（`mcp>=1.1,<2`，FastMCP）；2.x 在部分宿主上会「live tool discovery」失败。  
4. 终端自检：

```bash
./scripts/run_mcp.sh
# 或
.venv/bin/python -m sc_search.mcp_server
```

能启动（stdio 等待）说明服务本身正常，剩下是客户端重载 / 重新启用连接器。

**Q：查询结果是空的？**  
先跑 `python -m sc_search.ingest status`，确认对应源是 `ok`。空库时先 `init` 再 `sync`。

**Q：我想加自己的俗称？**  
编辑 `data/seed_aliases.json`，再执行：

```bash
python -m sc_search.ingest sync aliases
```

---

## 给开发者 / Agent 的文档

- 项目规则与检索习惯：[`AGENTS.md`](AGENTS.md)
- 站点清单：[`SOURCES.md`](SOURCES.md)
- 实战技巧沉淀：[`TIPS.md`](TIPS.md)
- 设计说明：[`docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md`](docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md)

---

## 许可证

本仓库**代码与自有文档**以 [MIT License](LICENSE) 发布。

第三方数据需单独遵守其原许可（同步到本地库时不会随本仓库分发 SQLite 数据）：

| 来源 | 用途 | 许可要点 |
| :--- | :--- | :--- |
| [汉化组 global.ini](https://citizenwiki.cn/Localization) | `sync zh` 标准中文译名 | **CC BY-NC-SA 4.0**，仅个人非商用 |
| UEX / SCMDB / Star Citizen Wiki 等 | 物价、任务、飞船参数、摘要 | 以各站点条款为准；本工具仅作个人游戏辅助 |

本项目与 Cloud Imperium Games / RSI 无官方关联；数据来自社区站点，仅供个人学习与游戏辅助。
