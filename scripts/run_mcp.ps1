# Launch sc-search MCP over stdio (Go / Windows).
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
if (-not $env:SC_SEARCH_DB) {
  $env:SC_SEARCH_DB = Join-Path $Root "data\sc_knowledge.sqlite"
}
Set-Location $Root

$GoBin = Join-Path $Root "go\sc-search.exe"
if (-not (Test-Path $GoBin)) {
  $Alt = Join-Path $Root "go\sc-search"
  if (Test-Path $Alt) { $GoBin = $Alt }
}
if (-not (Test-Path $GoBin)) {
  if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Error "sc-search MCP: missing Go binary and go toolchain"
  }
  Push-Location (Join-Path $Root "go")
  go build -o sc-search.exe ./cmd/sc-search
  Pop-Location
  $GoBin = Join-Path $Root "go\sc-search.exe"
}
& $GoBin mcp
exit $LASTEXITCODE
