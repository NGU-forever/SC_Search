# Wire MCP configs for a ready tree (Release zip or already-built checkout).
# Does NOT build Go and does NOT sync/write the knowledge DB.
# Writes: config/mcp_config.json (from example + absolute paths), .mcp.json, .cursor/mcp.json
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$Bin = Join-Path $Root "go\sc-search.exe"
if (-not (Test-Path $Bin)) {
  $Alt = Join-Path $Root "go\sc-search"
  if (Test-Path $Alt) {
    $Bin = $Alt
  } else {
    Write-Error "缺少可执行文件: go\sc-search(.exe)。请使用 Release 解压包，或先 go build。"
  }
}

$Db = Join-Path $Root "data\sc_knowledge.sqlite"
if (-not (Test-Path $Db)) {
  Write-Error "缺少数据库: $Db。Release 包应自带；开发树请先 .\scripts\sync_all.ps1"
}

$Py = Get-Command python3 -ErrorAction SilentlyContinue
if (-not $Py) {
  $Py = Get-Command python -ErrorAction SilentlyContinue
}
if (-not $Py) {
  Write-Error "需要 python3/python 以渲染 config/mcp_config.json"
}

& $Py.Source (Join-Path $Root "scripts\render_mcp_config.py") $Root $Bin $Db
if ($LASTEXITCODE -ne 0) {
  Write-Error "render_mcp_config.py failed with exit $LASTEXITCODE"
}

Write-Host ""
Write-Host "就绪（未编译、未改库）。请在客户端重载 MCP / 新开对话后再提问。"
Write-Host "  权威配置: $(Join-Path $Root 'config\mcp_config.json')"
Write-Host "  客户端 hints: cursor / claude_code / claude_desktop / chatgpt / deepseek / kimi / zcode / antigravity / doubao / …"
Write-Host "  二进制: $Bin"
Write-Host "  数据库: $Db"
