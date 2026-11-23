# 一键初始化并同步 SC Search 本地知识库（Go / Windows）
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$Bin = Join-Path $Root "go\sc-search.exe"
if (-not (Test-Path $Bin)) {
  $Alt = Join-Path $Root "go\sc-search"
  if (Test-Path $Alt) { $Bin = $Alt }
}
if (-not (Test-Path $Bin)) {
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "未找到 go 与 go\sc-search。请安装 Go 后重试。"
  }
  Push-Location (Join-Path $Root "go")
  go build -o sc-search.exe ./cmd/sc-search
  Pop-Location
  $Bin = Join-Path $Root "go\sc-search.exe"
}

Write-Host "==> 使用: $Bin"
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
  Write-Host "==> sc-search $($Args -join ' ')"
  & $Bin @Args
  if ($LASTEXITCODE -ne 0) { throw "命令失败: sc-search $($Args -join ' ')" }
}

Invoke-Ingest ingest init

try {
  Invoke-Ingest ingest sync uex
} catch {
  Write-Host "（跳过）uex 同步失败——若只需任务/飞船/中文，可忽略；查物价请配置 UEX_API_TOKEN"
}

Invoke-Ingest ingest sync scmdb live
Invoke-Ingest ingest sync scw
Invoke-Ingest ingest sync wiki
Invoke-Ingest ingest sync aliases

try {
  Invoke-Ingest ingest sync localization
} catch {
  Write-Host "（跳过）localization 同步失败——可稍后重试；若 data/raw 已有 global.*.ini 缓存，一般会自动回退使用"
}

Write-Host ""
Write-Host "==> 同步完成，最近状态："
Invoke-Ingest ingest status

Write-Host ""
Write-Host "完成。请在 MCP 客户端中启用 / 重载 sc-search 后再提问。"
