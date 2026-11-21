# SC Search — 星际公民本地问答助手

在 Cursor（或其它支持 MCP 的 AI 客户端）里，用**本地数据库**快速查《星际公民》物价、任务/蓝图、飞船参数和中英俗称，不必每次都翻网页。

适合：边玩边问「黄金哪卖贵」「大白鲨载货多少」「这个任务给什么蓝图」这类事实问题。

---

## 你需要准备什么

| 项目 | 说明 |
| :--- | :--- |
| 电脑 | macOS / Windows / Linux 均可 |
| Python | **3.11 或更高** |
| AI 客户端 | 推荐 [Cursor](https://cursor.com)，需能配置 MCP |
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

### 4. 接到 Cursor

1. 打开或编辑项目里的 `.cursor/mcp.json`。
2. 把路径改成**你自己电脑上的项目路径**（下面是示例）：

```json
{
  "mcpServers": {
    "sc-search": {
      "command": "/你的路径/SC_Search/.venv/bin/python",
      "args": ["-m", "sc_search.mcp_server"],
      "cwd": "/你的路径/SC_Search",
      "env": {
        "SC_SEARCH_DB": "/你的路径/SC_Search/data/sc_knowledge.sqlite"
      }
    }
  }
}
```

Windows 注意：`command` 一般写成  
`C:\\你的路径\\SC_Search\\.venv\\Scripts\\python.exe`。

3. 在 Cursor 里**重载 MCP / 重启 Agent**，确认出现服务器 **`sc-search`**。

### 5. 开始提问

回答链路：Agent 理解意图与槽位 → `sc-search` MCP 检索证据 → Agent 依据证据成文（简单题可直接 `sc_answer`）。

在对话里直接问即可。Agent 应优先调用本项目的工具，例如：

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

**Q：MCP 里看不到 sc-search？**  
检查 `.cursor/mcp.json` 路径是否指向本机 `.venv` 里的 Python；改完后重载 MCP。可在终端先跑：

```bash
.venv/bin/python -m sc_search.mcp_server
```

能启动（stdio 等待）说明包装好了。

**Q：查询结果是空的？**  
先跑 `python -m sc_search.ingest status`，确认对应源是 `ok`。空库时先 `init` 再 `sync`。

**Q：我想加自己的俗称？**  
编辑 `data/seed_aliases.json`（或打包内同名文件），再执行：

```bash
python -m sc_search.ingest sync aliases
```

---

## 给开发者 / Agent 的文档

- 项目规则与检索习惯：[`AGENTS.md`](AGENTS.md)
- 站点清单：[`SOURCES.md`](SOURCES.md)
- 实战技巧沉淀：[`TIPS.md`](TIPS.md)
- 设计说明：[`docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md`](docs/superpowers/specs/2026-11-21-sc-knowledge-sqlite-design.md)

本项目与 Cloud Imperium Games / RSI 无官方关联；数据来自社区站点，仅供个人学习与游戏辅助。
