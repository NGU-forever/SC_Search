# Launch sc-search MCP over stdio (Windows). Resolves repo root from this script.
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$Py = Join-Path $Root ".venv\Scripts\python.exe"
if (-not (Test-Path $Py)) {
  Write-Error "sc-search MCP: missing $Py (create .venv and pip install -e .)"
  exit 1
}
if (-not $env:SC_SEARCH_DB) {
  $env:SC_SEARCH_DB = Join-Path $Root "data\sc_knowledge.sqlite"
}
Set-Location $Root
& $Py -m sc_search.mcp_server
