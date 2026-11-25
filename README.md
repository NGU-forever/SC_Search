# SC Search — 星际公民本地问答助手

在 [豆包](https://www.doubao.com/)（豆包工作）或 Cursor 等支持 MCP 的 AI 客户端里，用**本地数据库**查《星际公民》物价、任务/蓝图、飞船参数和中英俗称。

普通用户：**下载 Release zip 即可，不必安装 Go。** 包内已含可执行文件和知识库。

---

## 使用教程（推荐）

### 1. 下载对应系统的包

打开 [Releases](https://github.com/NGU-forever/archangel-mcp/releases)，按电脑选择 zip：

| 你的系统 | 下载文件名大致为 |
| :--- | :--- |
| Apple Silicon Mac（M 系列） | `sc-search-*-darwin-arm64.zip` |
| Intel Mac | `sc-search-*-darwin-amd64.zip` |
| Windows 64 位 | `sc-search-*-windows-amd64.zip` |
| Linux x86_64 | `sc-search-*-linux-amd64.zip` |
| Linux ARM64 | `sc-search-*-linux-arm64.zip` |

不确定 Mac 芯片：左上角苹果菜单 →「关于本机」；芯片含 “Apple” 选 `arm64`，否则选 `amd64`。

### 2. 解压

解压到任意目录，例如 `~/Apps/sc-search-0.1.0-darwin-arm64/`。  
解压后应能看到：

```text
go/sc-search          # Windows 为 go/sc-search.exe
data/sc_knowledge.sqlite
scripts/setup_mcp.sh  # Windows 为 setup_mcp.ps1
AGENTS.md
README.md
```

**不要删 `data/sc_knowledge.sqlite`**——问答数据都在这里。

### 3. 接到 AI 客户端

#### 方式 A：有 Cursor / Claude Code 等 Agent（最省事）

1. 用客户端 **打开解压后的整个文件夹**（不要只开某个子文件）。
2. 对新对话发送：

```text
初始化
```

3. Agent 运行 `setup_mcp`，生成 [`config/mcp_config.json`](config/mcp_config.example.json)（本机绝对路径 + 各客户端 hints），并按**当前** AI 客户端自行写入/展示 MCP；**不会编译、不会重写数据库**。
4. 按提示 **重载 MCP** 或新开对话，即可提问。

#### 方式 B：手动脚本（无 Agent 时）

macOS / Linux：

```bash
cd /解压后的目录
chmod +x scripts/*.sh
./scripts/setup_mcp.sh
```

Windows PowerShell：

```powershell
cd X:\解压后的目录
.\scripts\setup_mcp.ps1
```

然后在客户端重载 MCP。

#### 方式 C：豆包工作（自定义连接器）

1. 先完成上面的脚本，或自己记下两个绝对路径：
   - 命令：`…/go/sc-search`（Windows：`…\go\sc-search.exe`）
   - 数据库：`…/data/sc_knowledge.sqlite`
2. 豆包 → **技能 · 连接器 · 伙伴** → **新建自定义连接器**（选**本地电脑**，不要选云电脑）：

| 字段 | 填写 |
| :--- | :--- |
| 服务器名称 | `sc-search` |
| 传输类型 | **STDIO** |
| 命令 | `go/sc-search` 的**绝对路径** |
| 参数 | `mcp` |
| 环境变量 | `SC_SEARCH_DB` = `data/sc_knowledge.sqlite` 的**绝对路径** |

3. 保存并启用连接器后开始提问。

### 4. 可以怎么问

直接用自然语言即可，例如：

- 「黄金在哪卖贵？」
- 「大白鲨是哪艘船？」
- 「Carrack 载货量和厂家？」
- 「红风 Junior 能接什么任务？」
- 「本地数据是什么时候同步的？」

编排细节见 [ORCHESTRATION.md](ORCHESTRATION.md)。常见工具对应关系：

| 你想问的 | 大致会用到的工具 |
| :--- | :--- |
| 简单事实/物价 | `sc_answer` |
| 复杂题 | `sc_route` → `sc_worker_run` |
| 最高卖价/最低买价 | `sc_best_prices` |
| 中英俗称解析 | `sc_resolve_name` |
| 飞船参数 | `sc_entity_facts` |
| 任务/蓝图列表 | `sc_list_missions` |
| 数据新鲜度 | `sc_data_freshness` |

### 5. 游戏大版本更新后（可选）

Release 包里的库是发布时的快照。要刷新到最新：

```bash
# 可选：查实时物价时配置 Token
cp config/.env.example config/.env
# 编辑 config/.env，填入 UEX_API_TOKEN（https://uexcorp.space/api/apps）

./scripts/sync_all.sh
# Windows: .\scripts\sync_all.ps1
```

用的是包内已有二进制，**仍然不必安装 Go**。MCP 不会自动改库。

---

## 你需要准备什么

| 项目 | 说明 |
| :--- | :--- |
| 电脑 | macOS / Windows / Linux |
| Go | **普通用户不需要**；仅从源码开发时要 1.22+ |
| AI 客户端 | 豆包 / Cursor / Claude / ChatGPT·Codex / DeepSeek / Kimi / ZCode / Antigravity 等支持本地 STDIO MCP 的客户端（见 `config/mcp_config.example.json` 的 `client_hints`） |
| UEX（可选） | 包内已有物价快照；要自己刷实时价再配 Token |

---

## 开发者：从源码构建

克隆本仓库后，在项目根目录：

```bash
cd go && go build -o sc-search ./cmd/sc-search && cd ..
cp config/.env.example config/.env   # 可选：填 UEX_API_TOKEN
chmod +x scripts/*.sh
./scripts/sync_all.sh
./scripts/setup_mcp.sh
```

对 Agent 说「初始化」亦可（开发树会走构建 + 同步）。

### 维护者发版

```bash
./scripts/package_release.sh --version 0.1.0
# 或：git tag v0.1.0 && git push origin v0.1.0
# → GitHub Actions 多平台打包并上传 Release（含预同步 SQLite）
```

---

## 现在能查什么 / 还不能指望什么

**本地库比较擅长：** 跑商价、飞船 aUEC 购点（UEX）、任务/蓝图（SCMDB）、飞船参数（SCW）、Wiki 摘要、汉化组译名、民间俗称。

**仍建议开网页确认：** Erkul 配装/DPS、采矿计算、Spectrum、需登录官方内容。

---

## 常见问题

**Q：一定要装 Go 吗？**  
不要。下 Release → 解压 → `setup_mcp` / 「初始化」即可。

**Q：zip 里有数据库吗？**  
有。`data/sc_knowledge.sqlite` 已打进包，解压就能查。

**Q：MCP 里看不到 sc-search？**  
确认命令是 `go/sc-search` 的绝对路径、参数为 `mcp`，并重载连接器。自检：`./scripts/run_mcp.sh`。

**Q：没有 UEX Token 能用吗？**  
能。Release 快照已可用；刷新实时价时再配 Token。

**Q：查询结果是空的？**  
`./go/sc-search ingest status` 看库是否正常。开发树空库需先 `sync_all`。

**Q：我想加俗称？**  
编辑 `data/seed_aliases.json`，再 `./go/sc-search ingest sync aliases`。

---

## 给开发者 / Agent

- Agent 入口：[`AGENTS.md`](AGENTS.md)
- 问答编排：[`ORCHESTRATION.md`](ORCHESTRATION.md)
- Go 状态机编排设计：[`docs/state_machine_orchestration_design.md`](docs/state_machine_orchestration_design.md)
- Go 说明：[`go/README.md`](go/README.md)
- 站点清单：[`SOURCES.md`](SOURCES.md)
- 测试：`cd go && go test ./...`

---

## 许可证

本仓库**代码与自有文档**以 [MIT License](LICENSE) 发布。

| 来源 | 用途 | 许可要点 |
| :--- | :--- | :--- |
| [汉化组 global.ini](https://citizenwiki.cn/Localization) | `sync localization` 标准中文译名 | **CC BY-NC-SA 4.0**，仅个人非商用 |
| UEX / SCMDB / Star Citizen Wiki 等 | 物价、任务、飞船参数、摘要 | 以各站点条款为准 |

本项目与 Cloud Imperium Games / RSI 无官方关联；数据来自社区站点，仅供个人学习与游戏辅助。
