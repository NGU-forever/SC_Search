# Launch sc-search MCP over stdio (Windows). Prefers Go binary; falls back to Python.
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
if (-not $env:SC_SEARCH_DB) {
  $env:SC_SEARCH_DB = Join-Path $Root "data\sc_knowledge.sqlite"
}
Set-Location $Root

$GoBin = Join-Path $Root "go\sc-search.exe"
if (-not (Test-Path $GoBin)) {
  $GoBinUnix = Join-Path $Root "go\sc-search"
  if (Test-Path $GoBinUnix) { $GoBin = $GoBinUnix }
}
if (-not (Test-Path $GoBin)) {
  if (Get-Command go -ErrorAction SilentlyContinue) {
    Push-Location (Join-Path $Root "go")
    go build -o sc-search.exe ./cmd/sc-search
    Pop-Location
    $GoBin = Join-Path $Root "go\sc-search.exe"
  }
}
if (Test-Path $GoBin) {
  & $GoBin mcp
  exit $LASTEXITCODE
}

$Py = Join-Path $Root ".venv\Scripts\python.exe"
if (-not (Test-Path $Py)) {
  Write-Error "sc-search MCP: missing Go binary and $Py"
  exit 1
}
Write-Warning "sc-search MCP: Go binary missing; falling back to Python"
& $Py -m sc_search.mcp_server
