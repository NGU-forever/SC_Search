# 一键初始化并同步 SC Search 本地知识库（Windows PowerShell）
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$Py = Join-Path $Root ".venv\Scripts\python.exe"
if (-not (Test-Path $Py)) {
  $Py = "python"
}

Write-Host "==> 使用: $Py"
Write-Host "==> 项目: $Root"

$EnvFile = Join-Path $Root "config\.env"
$EnvExample = Join-Path $Root "config\.env.example"
if (-not (Test-Path $EnvFile) -and (Test-Path $EnvExample)) {
  Copy-Item $EnvExample $EnvFile
  Write-Host "已创建 config\.env（可填入 UEX_API_TOKEN 以同步物价）"
}

function Invoke-Ingest {
  param([Parameter(ValueFromRemainingArguments = $true)]$Args)
  Write-Host ""
  Write-Host "==> python -m sc_search.ingest $($Args -join ' ')"
  & $Py -m sc_search.ingest @Args
  if ($LASTEXITCODE -ne 0) { throw "命令失败: sc_search.ingest $($Args -join ' ')" }
}

Invoke-Ingest init

try {
  Invoke-Ingest sync uex
} catch {
  Write-Host "（跳过）uex 同步失败——若只需任务/飞船/中文，可忽略；查物价请配置 UEX_API_TOKEN"
}

Invoke-Ingest sync scmdb --channel live
Invoke-Ingest sync scw
Invoke-Ingest sync wiki
Invoke-Ingest sync aliases

try {
  Invoke-Ingest sync zh
} catch {
  Write-Host "（跳过）zh 同步失败——可稍后重试；若 data/raw 已有 global.*.ini 缓存，一般会自动回退使用"
}

Write-Host ""
Write-Host "==> 同步完成，最近状态："
Invoke-Ingest status

Write-Host ""
Write-Host "完成。请在豆包（或其它 MCP 客户端）中启用 / 重载 sc-search 后再提问。"
