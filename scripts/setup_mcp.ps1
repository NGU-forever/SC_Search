# Wire MCP configs for a ready tree (Release zip or already-built checkout).
# Does NOT build Go and does NOT sync/write the knowledge DB.
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

function Write-McpJson([string]$Dest) {
  $dir = Split-Path -Parent $Dest
  if ($dir -and -not (Test-Path $dir)) {
    New-Item -ItemType Directory -Path $dir | Out-Null
  }
  $binJson = $Bin -replace '\\', '/'
  $rootJson = $Root -replace '\\', '/'
  $dbJson = $Db -replace '\\', '/'
  @"
{
  "mcpServers": {
    "sc-search": {
      "command": "$binJson",
      "args": ["mcp"],
      "cwd": "$rootJson",
      "env": {
        "SC_SEARCH_DB": "$dbJson"
      }
    }
  }
}
"@ | Set-Content -Path $Dest -Encoding utf8
  Write-Host "已写入 $Dest"
}

Write-McpJson (Join-Path $Root ".mcp.json")
Write-McpJson (Join-Path $Root ".cursor\mcp.json")

Write-Host ""
Write-Host "就绪（未编译、未改库）。请在客户端重载 MCP / 新开对话后再提问。"
Write-Host "  二进制: $Bin"
Write-Host "  数据库: $Db"
Write-Host "豆包等工作端：STDIO，命令=$Bin，参数=mcp，环境变量 SC_SEARCH_DB=$Db"
